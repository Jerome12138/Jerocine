package mysql

import (
	"context"
	"errors"
	"log"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

type searchRepo struct{ db *gorm.DB }

// NewSearchRepository 构造物化卡片/检索宽表仓储。
func NewSearchRepository(db *gorm.DB) repository.SearchRepository { return &searchRepo{db: db} }

// 排序键 —— 白名单(filter 的 sort 参数)、分类页三榜、首页全站热榜共用同一处定义, 避免两处漂移。
//
//	orderHot    热度优先: 主键是实测的 hot_score(见 service/hot_service.go), 后三级只做同分 tiebreak。
//	            列顺序与 000020 建的 idx_hot(hot_score, year, update_stamp, mid) 完全一致 → 走索引不 filesort。
//	orderScore  评分优先: db_score 无索引(存量如此), 但只取前 N + 分类页 10 分钟缓存, 可接受。
//	orderLatest 最新上线: 原为 release_stamp(= 源站录入时间, 受 2023 老片补录污染), 改用
//	            year + pub_date(源站上映日期, 精度自描述且字典序=时间序), 末级才回退 update_stamp。
//	orderRecent 最近更新: 源站最近动过它的时间。
const (
	orderHot    = "hot_score DESC, year DESC, update_stamp DESC, mid DESC"
	orderScore  = "db_score DESC, year DESC, mid DESC"
	orderLatest = "year DESC, pub_date DESC, update_stamp DESC"
	orderRecent = "update_stamp DESC"
)

// allowedSort 排序列白名单, 防注入。旧值(hits / db_score / release_stamp)保留为别名:
// 前端与 Android TV 已发出的链接、以及 TV 端硬编码的旧 value 都要继续工作。
var allowedSort = map[string]string{
	"hot":           orderHot,
	"hits":          orderHot, // 旧值(人气排序)
	"score":         orderScore,
	"db_score":      orderScore, // 旧值(评分排序)
	"latest":        orderLatest,
	"release_stamp": orderLatest, // 旧值(最新上映)
	"update_stamp":  orderRecent,
	"recent":        orderRecent,
}

// defaultSort 未指定 / 无法识别时的排序键。
const defaultSort = orderRecent

func orderForClassify(s repository.ClassifySort) string {
	switch s {
	case repository.SortLatest:
		return orderLatest
	case repository.SortHot:
		return orderHot
	case repository.SortScore:
		return orderScore
	default:
		return orderRecent
	}
}

// applyDeleted 追加软删态条件。公开读路径一律传零值(仅未删), 后台回收站传 DeletedOnly。
func applyDeleted(q *gorm.DB, mode int) *gorm.DB {
	switch mode {
	case repository.DeletedOnly:
		return q.Where("deleted_at > 0")
	case repository.DeletedInclude:
		return q // 不限(后台"全部")
	default:
		return q.Where("deleted_at = 0")
	}
}

// searchUpsertCols 采集回写读模型时参与 ON DUPLICATE KEY UPDATE 的列。
// 与 movieUpsertCols 同口径: 排除 mid(冲突键)、created_at(首存时间)、deleted_at(软删标记),
// 保证源站重推不会让已删影片在列表/检索里复活; db_score / year / pub_date / hot_score 走条件更新
// (见 base.go 的 conditionalUpsert), 源站给不出时保留本地值。
// 由 TestSearchUpsertColsCoverEntity 反射校验。
var searchUpsertCols = []string{
	"cid", "pid", "name", "sub_title", "c_name", "class_tag",
	"area", "language", "year", "pub_date", "initial", "name_pinyin", "state", "remarks",
	"db_score", "hits", "hot_score", "cover", "release_stamp", "update_stamp", "updated_at",
}

// searchUpsertExclude 见 searchUpsertCols 注释。backdrop 由 TMDB worker 双写回填, 采集不覆盖;
// hot_rank / hot_rank_at / hot_board / db_id_src 由豆瓣榜单任务写入, 同属"本地计算列"(hot_score 例外:
// 它随年份/在播/口碑变化, 走 IF(hot_rank = 0, ...) 条件更新, 在榜行不会被抹)。
var searchUpsertExclude = map[string]bool{
	"mid": true, "created_at": true, "deleted_at": true, "backdrop": true,
	"hot_rank": true, "hot_rank_at": true, "hot_board": true, "db_id_src": true,
}

// searchUpsertClause 冲突时按内容列更新(常规列 VALUES(col) + 条件更新列)。
func searchUpsertClause() clause.OnConflict {
	return clause.OnConflict{
		Columns:   []clause.Column{{Name: "mid"}},
		DoUpdates: upsertAssignments(searchUpsertCols),
	}
}

func (r *searchRepo) GetByMid(ctx context.Context, mid int64) (*entity.MovieSearch, error) {
	var m entity.MovieSearch
	err := applyDeleted(dbFrom(ctx, r.db), repository.DeletedExclude).Where("mid = ?", mid).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// CountCreatedSince 统计 created_at(毫秒) >= sinceMillis 的影片数(仪表盘今日/近一周新增, 不计已删)。
func (r *searchRepo) CountCreatedSince(ctx context.Context, sinceMillis int64) (int64, error) {
	var n int64
	err := applyDeleted(dbFrom(ctx, r.db).Model(&entity.MovieSearch{}), repository.DeletedExclude).
		Where("created_at >= ?", sinceMillis).Count(&n).Error
	return n, err
}

func (r *searchRepo) GetByMids(ctx context.Context, mids []int64) ([]entity.MovieSearch, error) {
	if len(mids) == 0 {
		return nil, nil
	}
	var out []entity.MovieSearch
	if err := applyDeleted(dbFrom(ctx, r.db), repository.DeletedExclude).Where("mid IN ?", mids).Find(&out).Error; err != nil {
		return nil, err
	}
	// 按入参 mids 顺序回排 (保持推荐/批量取的顺序)
	idx := make(map[int64]entity.MovieSearch, len(out))
	for _, m := range out {
		idx[m.Mid] = m
	}
	ordered := make([]entity.MovieSearch, 0, len(out))
	for _, id := range mids {
		if m, ok := idx[id]; ok {
			ordered = append(ordered, m)
		}
	}
	return ordered, nil
}

func (r *searchRepo) TopByPidSorted(ctx context.Context, pid int64, s repository.ClassifySort, limit int) ([]entity.MovieSearch, error) {
	if limit <= 0 {
		limit = 14
	}
	var out []entity.MovieSearch
	err := applyDeleted(dbFrom(ctx, r.db), repository.DeletedExclude).
		Where("pid = ?", pid).Order(orderForClassify(s)).Limit(limit).Find(&out).Error
	return out, err
}

// TopHotAll 全站跨类别热榜 —— 首页「热门榜单」行(不按 pid)。与分类页排行榜共用 orderHot,
// 但口径不同: 这里混排全站, 那里只看该分类(见 docs/榜单热度方案 §8.1①)。
func (r *searchRepo) TopHotAll(ctx context.Context, limit int) ([]entity.MovieSearch, error) {
	if limit <= 0 {
		limit = 14
	}
	var out []entity.MovieSearch
	err := applyDeleted(dbFrom(ctx, r.db), repository.DeletedExclude).
		Order(orderHot).Limit(limit).Find(&out).Error
	return out, err
}

// TopScoreByPid 该一级分类的高分榜。
//
// 口径(方案 §3.5): db_score > 0 且排除"解说" —— 用**片名内容**判定解说(实测与 c_name 口径
// 21,479 vs 21,502 等价), 不依赖任何分类 id: 切源重建分类树后判据依然成立。
// **不设分数下限**: 榜单本就只分页显示前面, 排序天然让高分在前, 门槛纯属多余
// (分类筛选页更不该被 ≥8 卡掉)。
func (r *searchRepo) TopScoreByPid(ctx context.Context, pid int64, limit int) ([]entity.MovieSearch, error) {
	if limit <= 0 {
		limit = 14
	}
	var out []entity.MovieSearch
	err := applyDeleted(dbFrom(ctx, r.db), repository.DeletedExclude).
		Where("pid = ? AND db_score > 0 AND name NOT LIKE ?", pid, "%解说%").
		Order(orderScore).Limit(limit).Find(&out).Error
	return out, err
}

// CountScoredByPid 该一级分类有评分的影片数 —— 分类页据此决定是否返回高分榜分区
// (为 0 则前端不渲染入口)。用运行时探测代替"体育/短剧/漫剧"白名单: 切源后自动正确。
func (r *searchRepo) CountScoredByPid(ctx context.Context, pid int64) (int64, error) {
	var n int64
	err := applyDeleted(dbFrom(ctx, r.db).Model(&entity.MovieSearch{}), repository.DeletedExclude).
		Where("pid = ? AND db_score > 0", pid).Count(&n).Error
	return n, err
}

func (r *searchRepo) Filter(ctx context.Context, spec repository.FilterSpec, page repository.Page) ([]entity.MovieSearch, int64, error) {
	q := applyDeleted(dbFrom(ctx, r.db).Model(&entity.MovieSearch{}), spec.Deleted)
	if spec.Pid > 0 {
		q = q.Where("pid = ?", spec.Pid)
	}
	if spec.Cid > 0 {
		q = q.Where("cid = ?", spec.Cid)
	}
	if spec.Area != "" {
		q = q.Where("area = ?", spec.Area)
	}
	if spec.Language != "" {
		q = q.Where("language = ?", spec.Language)
	}
	if spec.Year > 0 {
		q = q.Where("year = ?", spec.Year)
	}
	if spec.Plot != "" {
		q = q.Where("class_tag LIKE ?", "%"+spec.Plot+"%")
	}
	order := allowedSort[spec.Sort]
	if order == "" {
		order = defaultSort
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []entity.MovieSearch
	if err := q.Order(order).Limit(page.Limit()).Offset(page.Offset()).Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *searchRepo) SearchKeyword(ctx context.Context, keyword string, deleted int, page repository.Page) ([]entity.MovieSearch, int64, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, 0, nil
	}
	// FULLTEXT ngram 布尔模式, 替代前导通配 LIKE 全表扫。
	match := "MATCH(name, sub_title) AGAINST (? IN BOOLEAN MODE)"
	q := applyDeleted(dbFrom(ctx, r.db).Model(&entity.MovieSearch{}), deleted)
	if isAllAsciiLetters(keyword) {
		// 纯字母 → 可能是拼音首字母: 走 name_pinyin 前缀匹配, 同时兼容片名全文(英文名)。
		q = q.Where("name_pinyin LIKE ? OR "+match, strings.ToUpper(keyword)+"%", keyword)
	} else {
		q = q.Where(match, keyword)
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []entity.MovieSearch
	if err := q.Order("update_stamp DESC").Limit(page.Limit()).Offset(page.Offset()).Find(&out).Error; err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

// isAllAsciiLetters 关键字是否全为 ASCII 字母(可能是拼音首字母串)。
func isAllAsciiLetters(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return true
}

// BackfillNamePinyin 回填 movie_search.name_pinyin(仅空值且有片名的行), 让首字母搜索覆盖存量数据。
// film_api 启动时后台调用一次; 分批事务更新, 幂等(只挑空值行, 故重复调用安全)。
func BackfillNamePinyin(ctx context.Context, db *gorm.DB, batch int) (int, error) {
	if batch <= 0 {
		batch = 500
	}
	type row struct {
		Mid  int64
		Name string
	}
	total := 0
	for {
		var rows []row
		if err := db.WithContext(ctx).Model(&entity.MovieSearch{}).
			Select("mid", "name").
			Where("name_pinyin = '' AND name <> ''").
			Limit(batch).Find(&rows).Error; err != nil {
			return total, err
		}
		if len(rows) == 0 {
			break
		}
		err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			for _, x := range rows {
				py := domain.NameInitials(x.Name)
				if py == "" {
					py = "-" // 无可派生首字母(纯符号名): 占位, 避免反复命中空值条件死循环
				}
				if e := tx.Model(&entity.MovieSearch{}).Where("mid = ?", x.Mid).
					Update("name_pinyin", py).Error; e != nil {
					return e
				}
			}
			return nil
		})
		if err != nil {
			return total, err
		}
		total += len(rows)
		if len(rows) < batch {
			break
		}
	}
	return total, nil
}

func (r *searchRepo) Related(ctx context.Context, seed repository.RelatedSeed, candidateLimit int) ([]entity.MovieSearch, error) {
	if candidateLimit <= 0 {
		candidateLimit = 200
	}
	// 同分类 + (名称近似 OR 标签命中), 不再 ORDER BY RAND(); 随机抽样在 service 内存里做。
	q := applyDeleted(dbFrom(ctx, r.db).Model(&entity.MovieSearch{}), repository.DeletedExclude).
		Where("cid = ? AND mid <> ?", seed.Cid, seed.Mid)
	conds := dbFrom(ctx, r.db)
	hasCond := false
	if name := strings.TrimSpace(seed.Name); name != "" {
		conds = conds.Or("name LIKE ?", "%"+name+"%")
		hasCond = true
	}
	for _, tag := range splitTags(seed.ClassTag) {
		conds = conds.Or("class_tag LIKE ?", "%"+tag+"%")
		hasCond = true
	}
	if hasCond {
		q = q.Where(conds)
	}
	var out []entity.MovieSearch
	err := q.Limit(candidateLimit).Find(&out).Error
	return out, err
}

func (r *searchRepo) Upsert(ctx context.Context, m *entity.MovieSearch) error {
	fillHotScoreSearch(m, time.Now())
	return dbFrom(ctx, r.db).Clauses(searchUpsertClause()).Create(m).Error
}

func (r *searchRepo) BatchUpsert(ctx context.Context, list []entity.MovieSearch) error {
	if len(list) == 0 {
		return nil
	}
	now := time.Now()
	for i := range list {
		fillHotScoreSearch(&list[i], now)
	}
	return dbFrom(ctx, r.db).Clauses(searchUpsertClause()).CreateInBatches(list, 200).Error
}

func (r *searchRepo) Delete(ctx context.Context, mid int64) error {
	return dbFrom(ctx, r.db).Where("mid = ?", mid).Delete(&entity.MovieSearch{}).Error
}

func (r *searchRepo) SoftDelete(ctx context.Context, mid, deletedAt int64) error {
	if deletedAt <= 0 {
		deletedAt = nowMilli()
	}
	return dbFrom(ctx, r.db).Model(&entity.MovieSearch{}).
		Where("mid = ?", mid).
		Update("deleted_at", deletedAt).Error
}

func (r *searchRepo) Restore(ctx context.Context, mid int64) error {
	return dbFrom(ctx, r.db).Model(&entity.MovieSearch{}).
		Where("mid = ?", mid).
		Update("deleted_at", 0).Error
}

// UpdateBackdrop 回填横图读模型列(与 movie.backdrop 由 worker 双写同步)。
func (r *searchRepo) UpdateBackdrop(ctx context.Context, mid int64, url string) error {
	return dbFrom(ctx, r.db).Model(&entity.MovieSearch{}).
		Where("mid = ?", mid).
		Update("backdrop", url).Error
}

// SyncDeletedFromMovie 把 movie.deleted_at 回灌到 movie_search。
// 全量重采的读模型是从采集结果重建的(见 ShadowBegin/ShadowCommit), 新表里 deleted_at 全是 0,
// 若不回灌, 已删影片会集体回到列表/检索里。JOIN 更新, 只写真正有差异的行。
func (r *searchRepo) SyncDeletedFromMovie(ctx context.Context) error {
	return syncDeletedFrom(dbFrom(ctx, r.db), "movie_search")
}

// syncDeletedFrom 把主表的软删态回灌到指定读模型表。
// table 只由本包内的字面量传入, 不接受外部输入拼接, 无注入面。
func syncDeletedFrom(db *gorm.DB, table string) error {
	return db.Exec(
		"UPDATE " + table + " s JOIN movie m ON s.mid = m.mid " +
			"SET s.deleted_at = m.deleted_at WHERE s.deleted_at <> m.deleted_at",
	).Error
}

// ---- 全量重采无空窗影子表 ----

func (r *searchRepo) ShadowBegin(ctx context.Context) error {
	db := dbFrom(ctx, r.db)
	if err := db.Exec("DROP TABLE IF EXISTS movie_search_next").Error; err != nil {
		return err
	}
	return db.Exec("CREATE TABLE movie_search_next LIKE movie_search").Error
}

func (r *searchRepo) ShadowWrite(ctx context.Context, list []entity.MovieSearch) error {
	if len(list) == 0 {
		return nil
	}
	now := time.Now()
	for i := range list {
		fillHotScoreSearch(&list[i], now)
	}
	return dbFrom(ctx, r.db).Table("movie_search_next").CreateInBatches(list, 200).Error
}

// ShadowCommit 交换影子表, 让全量重采"无空窗"。
//
// 顺序是关键: **先**把软删态回灌进影子表, 再 RENAME 交换 —— 交换那一刻新表就已经是对的,
// 不存在"表已换、删除态还没补上"的窗口。旧实现是换表 + DROP 旧表之后才回灌, 那一步一旦失败
// 就回不去了(表已交换无法回滚): 已删影片会短暂出现在公开列表, 同时整轮采集被标成 error。
func (r *searchRepo) ShadowCommit(ctx context.Context) error {
	db := dbFrom(ctx, r.db)
	if err := syncDeletedFrom(db, "movie_search_next"); err != nil {
		return err
	}
	// 横图与删除态同理: 影子表由采集结果重建, backdrop 全是空, 换表前从 movie 回灌,
	// 否则一次全量重采就会把已回填的横图集体清掉。
	if err := db.Exec("UPDATE movie_search_next ns JOIN movie m ON m.mid = ns.mid " +
		"SET ns.backdrop = m.backdrop WHERE m.backdrop != ''").Error; err != nil {
		return err
	}
	// 榜单热度列同上。影子表里 hot_score 是**兜底分**(写入时按年份/在播/口碑算出来的, 见
	// fillHotScoreSearch), 但榜位(hot_rank / hot_rank_at / hot_board)与含榜位分的合成分只有
	// movie 表知道 —— 不回灌就等于"每跑一次全量, 全站榜位归零"。同样必须在 RENAME 之前完成。
	if err := db.Exec("UPDATE movie_search_next ns JOIN movie m ON m.mid = ns.mid " +
		"SET ns.hot_rank = m.hot_rank, ns.hot_rank_at = m.hot_rank_at, " +
		"ns.hot_board = m.hot_board, ns.hot_score = m.hot_score, ns.db_id_src = m.db_id_src " +
		"WHERE m.hot_score > 0 OR m.db_id_src > 0").Error; err != nil {
		return err
	}
	if err := db.Exec("RENAME TABLE movie_search TO movie_search_old, movie_search_next TO movie_search").Error; err != nil {
		return err
	}
	// 旧表只剩清理职责: 新表已是权威且已回灌, 这一步删不掉不影响一致性 —— 只告警, 不把
	// 已经成功的换表标成失败(否则整轮采集白跑)。
	if err := db.Exec("DROP TABLE IF EXISTS movie_search_old").Error; err != nil {
		log.Printf("search_repo: 清理 movie_search_old 失败(读模型已就绪, 不影响一致性): %v", err)
	}
	return nil
}

func (r *searchRepo) Truncate(ctx context.Context) error {
	db := dbFrom(ctx, r.db)
	db.Exec("DROP TABLE IF EXISTS movie_search_next") // 清理可能残留的影子表
	return db.Exec("TRUNCATE TABLE movie_search").Error
}

// ---- 7 维筛选标签 (查询期聚合, 替代采集期 ZIncrBy) ----

func (r *searchRepo) TagOptions(ctx context.Context, pid int64) (*repository.FilterOptions, error) {
	db := dbFrom(ctx, r.db)
	opts := &repository.FilterOptions{
		Titles: map[string]string{
			"Category": "类型", "Plot": "剧情", "Area": "地区", "Language": "语言",
			"Year": "年份", "Initial": "首字母", "Sort": "排序",
		},
		Tags:     map[string][]repository.TagOption{},
		SortList: []string{"Category", "Plot", "Area", "Language", "Year", "Sort"},
	}

	// Category: 该 pid 下展示中的子分类 (来自 category 表)
	type catRow struct {
		Id   int64
		Name string
	}
	var cats []catRow
	if err := db.Table("category").Select("id, name").
		Where("pid = ? AND `show` = 1", pid).Order("sort ASC, id ASC").Scan(&cats).Error; err != nil {
		return nil, err
	}
	cat := make([]repository.TagOption, 0, len(cats))
	for _, c := range cats {
		cat = append(cat, repository.TagOption{Name: c.Name, Value: strconv.FormatInt(c.Id, 10)})
	}
	opts.Tags["Category"] = cat

	// Plot: class_tag 组合去重后在内存拆分累加, 取 top10
	type cntRow struct {
		V string
		C int64
	}
	var combos []cntRow
	if err := applyDeleted(db.Model(&entity.MovieSearch{}), repository.DeletedExclude).
		Select("class_tag AS v, COUNT(*) AS c").
		Where("pid = ? AND class_tag <> ''", pid).Group("class_tag").Scan(&combos).Error; err != nil {
		return nil, err
	}
	plotCnt := map[string]int64{}
	for _, row := range combos {
		for _, tag := range splitTags(row.V) {
			plotCnt[tag] += row.C
		}
	}
	opts.Tags["Plot"] = topByCount(plotCnt, 10)

	// Area / Language: GROUP BY 取高频
	opts.Tags["Area"] = r.groupTop(db, pid, "area", 11)
	opts.Tags["Language"] = r.groupTop(db, pid, "language", 6)

	// Year: 存在的年份倒序
	var years []int
	if err := applyDeleted(db.Model(&entity.MovieSearch{}), repository.DeletedExclude).
		Where("pid = ? AND year > 0", pid).
		Distinct().Order("year DESC").Limit(12).Pluck("year", &years).Error; err != nil {
		return nil, err
	}
	yearOpts := make([]repository.TagOption, 0, len(years))
	for _, y := range years {
		s := strconv.Itoa(y)
		yearOpts = append(yearOpts, repository.TagOption{Name: s, Value: s})
	}
	opts.Tags["Year"] = yearOpts

	// Initial: 静态 A-Z
	initial := make([]repository.TagOption, 0, 26)
	for c := 'A'; c <= 'Z'; c++ {
		initial = append(initial, repository.TagOption{Name: string(c), Value: string(c)})
	}
	opts.Tags["Initial"] = initial

	// Sort: 静态。value 与 allowedSort 的键一一对应(web / Android TV 都从本接口取值, 不硬编码文案)。
	// "最新上映"曾是 release_stamp(= 源站录入时间, 受 2023 老片补录污染) —— 已换成 year + pub_date。
	opts.Tags["Sort"] = []repository.TagOption{
		{Name: "最近更新", Value: "update_stamp"},
		{Name: "热度优先", Value: "hot"},
		{Name: "评分优先", Value: "score"},
		{Name: "最新上线", Value: "latest"},
	}
	return opts, nil
}

func (r *searchRepo) groupTop(db *gorm.DB, pid int64, col string, limit int) []repository.TagOption {
	type cntRow struct {
		V string
		C int64
	}
	var rows []cntRow
	applyDeleted(db.Model(&entity.MovieSearch{}), repository.DeletedExclude).
		Select(col+" AS v, COUNT(*) AS c").
		Where("pid = ? AND "+col+" <> ''", pid).Group(col).Order("c DESC").Limit(limit).Scan(&rows)
	out := make([]repository.TagOption, 0, len(rows))
	for _, row := range rows {
		out = append(out, repository.TagOption{Name: row.V, Value: row.V})
	}
	return out
}

// splitTags 把 class_tag 按 , / ， 、 拆成标签列表。
func splitTags(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	repl := strings.NewReplacer("/", ",", "，", ",", "、", ",")
	parts := strings.Split(repl.Replace(s), ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func topByCount(m map[string]int64, n int) []repository.TagOption {
	type kv struct {
		k string
		v int64
	}
	arr := make([]kv, 0, len(m))
	for k, v := range m {
		arr = append(arr, kv{k, v})
	}
	sort.Slice(arr, func(i, j int) bool {
		if arr[i].v != arr[j].v {
			return arr[i].v > arr[j].v
		}
		return arr[i].k < arr[j].k
	})
	if len(arr) > n {
		arr = arr[:n]
	}
	out := make([]repository.TagOption, 0, len(arr))
	for _, e := range arr {
		out = append(out, repository.TagOption{Name: e.k, Value: e.k})
	}
	return out
}
