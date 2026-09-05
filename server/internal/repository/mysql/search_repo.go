package mysql

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

type searchRepo struct{ db *gorm.DB }

// NewSearchRepository 构造物化卡片/检索宽表仓储。
func NewSearchRepository(db *gorm.DB) repository.SearchRepository { return &searchRepo{db: db} }

// allowedSort 排序列白名单, 防注入。
var allowedSort = map[string]string{
	"update_stamp":  "update_stamp DESC",
	"hits":          "hits DESC",
	"db_score":      "db_score DESC",
	"release_stamp": "release_stamp DESC",
}

func orderForClassify(s repository.ClassifySort) string {
	switch s {
	case repository.SortLatest:
		return "release_stamp DESC"
	case repository.SortHot:
		return "hits DESC"
	default:
		return "update_stamp DESC"
	}
}

func (r *searchRepo) GetByMid(ctx context.Context, mid int64) (*entity.MovieSearch, error) {
	var m entity.MovieSearch
	err := dbFrom(ctx, r.db).Where("mid = ?", mid).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// CountCreatedSince 统计 created_at(毫秒) >= sinceMillis 的影片数。
func (r *searchRepo) CountCreatedSince(ctx context.Context, sinceMillis int64) (int64, error) {
	var n int64
	err := dbFrom(ctx, r.db).Model(&entity.MovieSearch{}).Where("created_at >= ?", sinceMillis).Count(&n).Error
	return n, err
}

func (r *searchRepo) GetByMids(ctx context.Context, mids []int64) ([]entity.MovieSearch, error) {
	if len(mids) == 0 {
		return nil, nil
	}
	var out []entity.MovieSearch
	if err := dbFrom(ctx, r.db).Where("mid IN ?", mids).Find(&out).Error; err != nil {
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
	err := dbFrom(ctx, r.db).Where("pid = ?", pid).Order(orderForClassify(s)).Limit(limit).Find(&out).Error
	return out, err
}

func (r *searchRepo) Filter(ctx context.Context, spec repository.FilterSpec, page repository.Page) ([]entity.MovieSearch, int64, error) {
	q := dbFrom(ctx, r.db).Model(&entity.MovieSearch{})
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
		order = "update_stamp DESC"
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

func (r *searchRepo) SearchKeyword(ctx context.Context, keyword string, page repository.Page) ([]entity.MovieSearch, int64, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return nil, 0, nil
	}
	// FULLTEXT ngram 布尔模式, 替代前导通配 LIKE 全表扫。
	match := "MATCH(name, sub_title) AGAINST (? IN BOOLEAN MODE)"
	q := dbFrom(ctx, r.db).Model(&entity.MovieSearch{})
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
	q := dbFrom(ctx, r.db).Model(&entity.MovieSearch{}).Where("cid = ? AND mid <> ?", seed.Cid, seed.Mid)
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
	return dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "mid"}},
		UpdateAll: true,
	}).Create(m).Error
}

func (r *searchRepo) BatchUpsert(ctx context.Context, list []entity.MovieSearch) error {
	if len(list) == 0 {
		return nil
	}
	return dbFrom(ctx, r.db).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "mid"}},
		UpdateAll: true,
	}).CreateInBatches(list, 200).Error
}

func (r *searchRepo) Delete(ctx context.Context, mid int64) error {
	return dbFrom(ctx, r.db).Where("mid = ?", mid).Delete(&entity.MovieSearch{}).Error
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
	return dbFrom(ctx, r.db).Table("movie_search_next").CreateInBatches(list, 200).Error
}

func (r *searchRepo) ShadowCommit(ctx context.Context) error {
	db := dbFrom(ctx, r.db)
	if err := db.Exec("RENAME TABLE movie_search TO movie_search_old, movie_search_next TO movie_search").Error; err != nil {
		return err
	}
	return db.Exec("DROP TABLE IF EXISTS movie_search_old").Error
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
	if err := db.Model(&entity.MovieSearch{}).Select("class_tag AS v, COUNT(*) AS c").
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
	if err := db.Model(&entity.MovieSearch{}).
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

	// Sort: 静态
	opts.Tags["Sort"] = []repository.TagOption{
		{Name: "时间排序", Value: "update_stamp"},
		{Name: "人气排序", Value: "hits"},
		{Name: "评分排序", Value: "db_score"},
		{Name: "最新上映", Value: "release_stamp"},
	}
	return opts, nil
}

func (r *searchRepo) groupTop(db *gorm.DB, pid int64, col string, limit int) []repository.TagOption {
	type cntRow struct {
		V string
		C int64
	}
	var rows []cntRow
	db.Model(&entity.MovieSearch{}).Select(col+" AS v, COUNT(*) AS c").
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
