package spider

import (
	"context"
	"log"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/platform/blobstore"
)

// Engine 采集引擎: 拉取 → 解析 → 事务双写(movie+movie_search+play); 全量走影子表无空窗。
type Engine struct {
	fetcher  *Fetcher
	movie    repository.MovieRepository
	search   repository.SearchRepository
	play     repository.PlaySourceRepository
	category repository.CategoryRepository
	files    repository.FileRepository
	tx       repository.TxManager
	blob     blobstore.BlobStore
	maxG     int
}

func NewEngine(
	m repository.MovieRepository, s repository.SearchRepository, p repository.PlaySourceRepository,
	c repository.CategoryRepository, f repository.FileRepository, tx repository.TxManager,
	blob blobstore.BlobStore, maxGoroutine int,
) *Engine {
	if maxGoroutine <= 0 {
		maxGoroutine = 32
	}
	return &Engine{
		fetcher: NewFetcher(), movie: m, search: s, play: p, category: c,
		files: f, tx: tx, blob: blob, maxG: maxGoroutine,
	}
}

// Collect 对单个采集源执行一次采集。hours<=0 全量(主站走影子表), hours>0 增量。
func (e *Engine) Collect(ctx context.Context, src *entity.CollectSource, hours int) (err error) {
	master := src.Grade == entity.GradeMaster
	cache.JobStart(ctx, src.Id, src.Name, 0)
	defer func() {
		if r := recover(); r != nil {
			log.Printf("spider Collect panic src=%s: %v", src.Id, r)
			cache.JobSetState(ctx, src.Id, cache.JobError)
		}
	}()

	// 主站: 分类缺失则先采分类
	if master {
		if cats, e2 := e.category.All(ctx); e2 == nil && len(cats) == 0 {
			if cl, e3 := e.fetcher.Categories(ctx, src); e3 == nil && len(cl) > 0 {
				if e4 := e.category.BatchUpsert(ctx, parseCategories(cl)); e4 == nil {
					cache.InvalidateCategory(ctx)
				}
			}
		}
	}

	pageCount, err := e.fetcher.PageCount(ctx, src, hours)
	if err != nil || pageCount <= 0 {
		if pageCount <= 0 && err == nil {
			cache.JobSetState(ctx, src.Id, cache.JobDone)
			return nil
		}
		cache.JobSetState(ctx, src.Id, cache.JobError)
		return err
	}
	cache.JobSetTotal(ctx, src.Id, pageCount)

	full := hours <= 0
	if master && full {
		if err = e.search.ShadowBegin(ctx); err != nil {
			cache.JobSetState(ctx, src.Id, cache.JobError)
			return err
		}
	}

	e.runPages(ctx, src, pageCount, hours, master, full)

	if master && full {
		if err = e.search.ShadowCommit(ctx); err != nil {
			cache.JobSetState(ctx, src.Id, cache.JobError)
			return err
		}
	}
	if master {
		cache.InvalidateAfterCollect(ctx)
	}
	if cache.JobReadState(ctx, src.Id) != cache.JobCanceled {
		cache.JobSetState(ctx, src.Id, cache.JobDone)
	}
	return nil
}

// runPages 并发分页采集; IntervalMs>500 的源退化为单线程限速。
func (e *Engine) runPages(ctx context.Context, src *entity.CollectSource, pageCount, hours int, master, full bool) {
	workers := e.maxG
	if workers > pageCount {
		workers = pageCount
	}
	throttled := src.IntervalMs > 500
	if throttled {
		workers = 1
	}

	pages := make(chan int, pageCount)
	for i := 1; i <= pageCount; i++ {
		pages <- i
	}
	close(pages)

	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					log.Printf("spider worker panic src=%s: %v", src.Id, r)
					cache.JobIncrFailed(ctx, src.Id, 1)
				}
			}()
			for pg := range pages {
				if !e.waitIfPausedOrCanceled(ctx, src.Id) {
					return // 已取消
				}
				if err := e.collectPage(ctx, src, pg, hours, master, full); err != nil {
					log.Printf("spider page src=%s pg=%d err=%v", src.Id, pg, err)
					cache.JobIncrFailed(ctx, src.Id, 1)
				} else {
					cache.JobIncrDone(ctx, src.Id, 1)
				}
				if throttled {
					time.Sleep(time.Duration(src.IntervalMs) * time.Millisecond)
				}
			}
		}()
	}
	wg.Wait()
}

// waitIfPausedOrCanceled 暂停则轮询等待; 取消返回 false。
func (e *Engine) waitIfPausedOrCanceled(ctx context.Context, jobId string) bool {
	for {
		switch cache.JobReadState(ctx, jobId) {
		case cache.JobCanceled:
			return false
		case cache.JobPaused:
			select {
			case <-ctx.Done():
				return false
			case <-time.After(time.Second):
			}
		default:
			return true
		}
	}
}

// collectPage 拉取并落库一页。
func (e *Engine) collectPage(ctx context.Context, src *entity.CollectSource, pg, hours int, master, full bool) error {
	if !master {
		return e.collectSlavePage(ctx, src, pg, hours)
	}
	return e.collectMasterPage(ctx, src, pg, hours, full)
}

func (e *Engine) collectMasterPage(ctx context.Context, src *entity.CollectSource, pg, hours int, full bool) error {
	movies := make([]entity.Movie, 0, 30)
	var plays []entity.MoviePlaySource
	if src.ResultModel == entity.ResultXML {
		vs, err := e.fetcher.XMLDetails(ctx, src, pg, hours)
		if err != nil {
			return err
		}
		for _, v := range vs {
			m := xmlToMovie(v)
			movies = append(movies, *m)
			plays = append(plays, xmlPlaySources(v, src.Id, &m.Mid)...)
		}
	} else {
		ds, err := e.fetcher.JSONDetails(ctx, src, pg, hours)
		if err != nil {
			return err
		}
		for _, d := range ds {
			m := toMovie(d)
			movies = append(movies, *m)
			plays = append(plays, buildPlaySources(d, src.Id, &m.Mid)...)
		}
	}
	return e.upsertMaster(ctx, src, movies, plays, full)
}

// upsertMaster 主站落库: 影片 + 检索(full 走影子表, 否则普通 upsert) + 多源, 事务双写; 封面 best-effort。
// 供分页全量/增量(collectMasterPage)与按片名采集(collectNamePage)复用。
func (e *Engine) upsertMaster(ctx context.Context, src *entity.CollectSource, movies []entity.Movie, plays []entity.MoviePlaySource, full bool) error {
	if len(movies) == 0 {
		return nil
	}
	// 图片同步: 下载封面到本地, 把 cover 改写为本地 URL(写库前完成, 列表/详情即用本地图)
	var fileRows []entity.FileInfo
	if src.SyncPictures && e.blob != nil {
		fileRows = e.downloadCovers(ctx, movies)
	}
	searchRows := make([]entity.MovieSearch, 0, len(movies))
	for i := range movies {
		searchRows = append(searchRows, *domain.ProjectMovieToSearch(&movies[i]))
	}
	err := e.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := e.movie.BatchUpsert(ctx, movies); err != nil {
			return err
		}
		if full {
			if err := e.search.ShadowWrite(ctx, searchRows); err != nil {
				return err
			}
		} else if err := e.search.BatchUpsert(ctx, searchRows); err != nil {
			return err
		}
		return e.play.UpsertBatch(ctx, plays)
	})
	if err != nil {
		return err
	}
	for i := range fileRows {
		_ = e.files.Create(ctx, &fileRows[i]) // 图片元数据 best-effort
	}
	return nil
}

func (e *Engine) collectSlavePage(ctx context.Context, src *entity.CollectSource, pg, hours int) error {
	var plays []entity.MoviePlaySource
	if src.ResultModel == entity.ResultXML {
		vs, err := e.fetcher.XMLDetails(ctx, src, pg, hours)
		if err != nil {
			return err
		}
		for _, v := range vs {
			plays = append(plays, xmlPlaySources(v, src.Id, nil)...)
		}
	} else {
		ds, err := e.fetcher.JSONDetails(ctx, src, pg, hours)
		if err != nil {
			return err
		}
		for _, d := range ds {
			plays = append(plays, buildPlaySources(d, src.Id, nil)...)
		}
	}
	return e.play.UpsertBatch(ctx, plays)
}

// SearchHit 按片名搜索的单条预览(只读, 不落库): 供新增页展示"某源有哪些匹配影片及集数"。
type SearchHit struct {
	SourceVodID int64  `json:"sourceVodId"`
	Name        string `json:"name"`
	Year        int    `json:"year"`
	TypeName    string `json:"typeName"`
	Remarks     string `json:"remarks"`
	Cover       string `json:"cover"`
	Episodes    int    `json:"episodes"` // 该源该片最长线路集数
}

// maxSearchPages 按片名采集最多翻几页(搜索结果通常 1 页, 留余量兜底)。
const maxSearchPages = 3

// SearchPreview 按片名搜索单个源, 返回匹配影片预览(只读, 不落库)。供新增页"搜索站点影片信息"。
func (e *Engine) SearchPreview(ctx context.Context, src *entity.CollectSource, wd string) ([]SearchHit, error) {
	var hits []SearchHit
	if src.ResultModel == entity.ResultXML {
		vs, err := e.fetcher.SearchXMLDetails(ctx, src, wd, 1)
		if err != nil {
			return nil, err
		}
		for _, v := range vs {
			hits = append(hits, SearchHit{
				SourceVodID: v.ID, Name: v.Name.Text, Year: parseYear(v.Year, v.Year),
				TypeName: v.Type, Remarks: v.Note.Text, Cover: v.Pic, Episodes: xmlEpisodeCount(v),
			})
		}
		return hits, nil
	}
	ds, err := e.fetcher.SearchJSONDetails(ctx, src, wd, 1)
	if err != nil {
		return nil, err
	}
	for _, d := range ds {
		hits = append(hits, SearchHit{
			SourceVodID: d.VodID, Name: d.VodName, Year: parseYear(d.VodPubDate, d.VodYear),
			TypeName: d.TypeName, Remarks: d.VodRemarks, Cover: d.VodPic, Episodes: jsonEpisodeCount(d),
		})
	}
	return hits, nil
}

// CollectByName 按片名采集单个源: 主站建/更新影片(非影子表, 增量式), 补充源按 match_key 挂源。
// 返回采集到的影片条数; 采到内容后失效内容缓存让前台即时可见。
func (e *Engine) CollectByName(ctx context.Context, src *entity.CollectSource, wd string) (int, error) {
	master := src.Grade == entity.GradeMaster
	total := 0
	for pg := 1; pg <= maxSearchPages; pg++ {
		n, err := e.collectNamePage(ctx, src, wd, pg, master)
		if err != nil {
			if pg == 1 {
				return total, err
			}
			break // 后续页失败不致命, 已采的算数
		}
		total += n
		if n == 0 {
			break // 无更多结果
		}
	}
	if total > 0 {
		cache.InvalidateAfterCollect(ctx)
	}
	return total, nil
}

// joinIds 把 vodId 列表拼成 maccms ids 参数(逗号分隔, 跳过非正数)。
func joinIds(ids []int64) string {
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		if id > 0 {
			parts = append(parts, strconv.FormatInt(id, 10))
		}
	}
	return strings.Join(parts, ",")
}

// CollectByIds 按 vodId 精确采集单个源指定影片(解决同名多版本: 名搜出候选后锁定具体那部)。
// 主站走 upsertMaster(full=false, 建/更新影片并挂源), 补充源仅 play.UpsertBatch(按 match_key 挂源)。
// 返回采集到的影片条数; 采到内容后失效内容缓存让前台即时可见。
func (e *Engine) CollectByIds(ctx context.Context, src *entity.CollectSource, ids []int64) (int, error) {
	idsStr := joinIds(ids)
	if idsStr == "" {
		return 0, nil
	}
	master := src.Grade == entity.GradeMaster
	n := 0
	if master {
		movies := make([]entity.Movie, 0, len(ids))
		var plays []entity.MoviePlaySource
		if src.ResultModel == entity.ResultXML {
			vs, err := e.fetcher.IdsXMLDetails(ctx, src, idsStr)
			if err != nil {
				return 0, err
			}
			for _, v := range vs {
				m := xmlToMovie(v)
				movies = append(movies, *m)
				plays = append(plays, xmlPlaySources(v, src.Id, &m.Mid)...)
			}
		} else {
			ds, err := e.fetcher.IdsJSONDetails(ctx, src, idsStr)
			if err != nil {
				return 0, err
			}
			for _, d := range ds {
				m := toMovie(d)
				movies = append(movies, *m)
				plays = append(plays, buildPlaySources(d, src.Id, &m.Mid)...)
			}
		}
		n = len(movies)
		if err := e.upsertMaster(ctx, src, movies, plays, false); err != nil {
			return 0, err
		}
	} else {
		// 补充源: 仅按 match_key 挂播放源(挂到已存在的影片上)
		var plays []entity.MoviePlaySource
		if src.ResultModel == entity.ResultXML {
			vs, err := e.fetcher.IdsXMLDetails(ctx, src, idsStr)
			if err != nil {
				return 0, err
			}
			n = len(vs)
			for _, v := range vs {
				plays = append(plays, xmlPlaySources(v, src.Id, nil)...)
			}
		} else {
			ds, err := e.fetcher.IdsJSONDetails(ctx, src, idsStr)
			if err != nil {
				return 0, err
			}
			n = len(ds)
			for _, d := range ds {
				plays = append(plays, buildPlaySources(d, src.Id, nil)...)
			}
		}
		if len(plays) > 0 {
			if err := e.play.UpsertBatch(ctx, plays); err != nil {
				return 0, err
			}
		}
	}
	if n > 0 {
		cache.InvalidateAfterCollect(ctx)
	}
	return n, nil
}

// collectNamePage 按片名采集一页(主站建影片+挂源; 补充源仅按 match_key 挂源), 返回该页影片数。
func (e *Engine) collectNamePage(ctx context.Context, src *entity.CollectSource, wd string, pg int, master bool) (int, error) {
	if master {
		movies := make([]entity.Movie, 0, 30)
		var plays []entity.MoviePlaySource
		if src.ResultModel == entity.ResultXML {
			vs, err := e.fetcher.SearchXMLDetails(ctx, src, wd, pg)
			if err != nil {
				return 0, err
			}
			for _, v := range vs {
				m := xmlToMovie(v)
				movies = append(movies, *m)
				plays = append(plays, xmlPlaySources(v, src.Id, &m.Mid)...)
			}
		} else {
			ds, err := e.fetcher.SearchJSONDetails(ctx, src, wd, pg)
			if err != nil {
				return 0, err
			}
			for _, d := range ds {
				m := toMovie(d)
				movies = append(movies, *m)
				plays = append(plays, buildPlaySources(d, src.Id, &m.Mid)...)
			}
		}
		return len(movies), e.upsertMaster(ctx, src, movies, plays, false)
	}
	// 补充源: 仅按 match_key 挂播放源(挂到已存在的影片上)
	var plays []entity.MoviePlaySource
	n := 0
	if src.ResultModel == entity.ResultXML {
		vs, err := e.fetcher.SearchXMLDetails(ctx, src, wd, pg)
		if err != nil {
			return 0, err
		}
		n = len(vs)
		for _, v := range vs {
			plays = append(plays, xmlPlaySources(v, src.Id, nil)...)
		}
	} else {
		ds, err := e.fetcher.SearchJSONDetails(ctx, src, wd, pg)
		if err != nil {
			return 0, err
		}
		n = len(ds)
		for _, d := range ds {
			plays = append(plays, buildPlaySources(d, src.Id, nil)...)
		}
	}
	if len(plays) == 0 {
		return n, nil
	}
	return n, e.play.UpsertBatch(ctx, plays)
}

// downloadCovers 并发下载封面到 BlobStore, 改写 movies[i].Cover 为本地 URL, 返回 files 元数据行。
func (e *Engine) downloadCovers(ctx context.Context, movies []entity.Movie) []entity.FileInfo {
	type job struct{ i int }
	jobs := make(chan job, len(movies))
	for i := range movies {
		if movies[i].Cover != "" {
			jobs <- job{i}
		}
	}
	close(jobs)
	rows := make([]entity.FileInfo, len(movies))
	ok := make([]bool, len(movies))
	var wg sync.WaitGroup
	workers := 8
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for j := range jobs {
				m := &movies[j.i]
				ext := coverExt(m.Cover)
				key := "poster/" + strconv.FormatInt(m.Mid, 10) + ext
				localURL, err := e.blob.SaveFromURL(ctx, key, m.Cover)
				if err != nil {
					continue
				}
				rows[j.i] = entity.FileInfo{
					Link: localURL, ObjectKey: key, RelevanceId: m.Mid,
					Type: entity.FileTypeCover, FileType: strings.TrimPrefix(ext, "."),
				}
				ok[j.i] = true
				m.Cover = localURL
			}
		}()
	}
	wg.Wait()
	out := make([]entity.FileInfo, 0)
	for i := range rows {
		if ok[i] {
			out = append(out, rows[i])
		}
	}
	return out
}

// CoverCategories 重新采集并覆盖分类(后台"分类覆盖"用)。
func (e *Engine) CoverCategories(ctx context.Context, src *entity.CollectSource) error {
	cl, err := e.fetcher.Categories(ctx, src)
	if err != nil {
		return err
	}
	if len(cl) == 0 {
		return nil
	}
	if err := e.category.BatchUpsert(ctx, parseCategories(cl)); err != nil {
		return err
	}
	cache.InvalidateCategory(ctx)
	return nil
}

// Zero 清空全部影片数据(详情/检索/多源), 并失效缓存。分类保留。
func (e *Engine) Zero(ctx context.Context) error {
	if err := e.search.Truncate(ctx); err != nil {
		return err
	}
	if err := e.movie.Truncate(ctx); err != nil {
		return err
	}
	if err := e.play.Truncate(ctx); err != nil {
		return err
	}
	cache.InvalidateAfterCollect(ctx)
	return nil
}

func coverExt(u string) string {
	ext := strings.ToLower(path.Ext(u))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
		return ext
	default:
		return ".jpg"
	}
}
