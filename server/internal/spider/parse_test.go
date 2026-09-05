package spider

import (
	"testing"

	"server/internal/domain"
)

func sampleDetail() filmDetail {
	return filmDetail{
		VodID: 7, TypeID: 11, TypeID1: 1, TypeName: "动作片", VodName: "测试电影",
		VodSub: "副标题", VodPic: "http://img/x.jpg", VodArea: "中国", VodLang: "国语",
		VodPubDate: "2023-05-01", VodYear: "2022", VodDouBanScore: "8.5", VodScore: "7.0",
		VodTime: "2023-05-01 12:00:00", VodTimeAdd: 1680000000, VodHits: 999, VodDouBanID: 100,
		VodPlayFrom: "snm3u8$$$kuaikan$$$yunpan",
		VodPlayNote: "$$$",
		VodPlayURL:  "第01集$http://a/1.m3u8#第02集$http://a/2.m3u8$$$第01集$http://b/1.m3u8$$$云盘$http://c/x.zip",
	}
}

func TestToMovie(t *testing.T) {
	m := toMovie(sampleDetail())
	if m.Mid != 7 || m.Cid != 11 || m.Pid != 1 || m.Name != "测试电影" {
		t.Fatalf("basic fields: %+v", m)
	}
	if m.Year != 2023 { // 优先 pubdate
		t.Fatalf("year = %d, want 2023", m.Year)
	}
	if m.DbScore != 8.5 {
		t.Fatalf("dbScore = %v, want 8.5", m.DbScore)
	}
	if m.UpdateStamp == 0 {
		t.Fatalf("update stamp should parse from VodTime")
	}
	if m.Cover != "http://img/x.jpg" {
		t.Fatalf("cover: %s", m.Cover)
	}
	if len(m.PlayFrom) != 3 {
		t.Fatalf("playFrom: %v", m.PlayFrom)
	}
}

func TestParsePlayGroups_ZipAndFilter(t *testing.T) {
	d := sampleDetail()
	groups := parsePlayGroups(d.VodPlayFrom, d.VodPlayURL, d.VodPlayNote)
	// 第三组(云盘 .zip)不含 m3u8/mp4 → 被过滤; 保留 snm3u8 / kuaikan 两组
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2: %+v", len(groups), groups)
	}
	if groups[0].Name != "snm3u8" || len(groups[0].Episodes) != 2 {
		t.Fatalf("group0: %+v", groups[0])
	}
	if groups[0].Episodes[1].Episode != "第02集" || groups[0].Episodes[1].Link != "http://a/2.m3u8" {
		t.Fatalf("episode parse: %+v", groups[0].Episodes[1])
	}
	if groups[1].Name != "kuaikan" {
		t.Fatalf("group1 name: %s", groups[1].Name)
	}
}

func TestBuildPlaySources_MasterVsSlave(t *testing.T) {
	d := sampleDetail()
	mid := int64(7)
	master := buildPlaySources(d, "src_lz", &mid)
	// 2 个 m3u8 组 × 1 key(片名) = 2 行, 均带 mid
	if len(master) != 2 {
		t.Fatalf("master sources = %d, want 2", len(master))
	}
	wantKey := domain.GenerateHashKey(d.VodName)
	for _, ps := range master {
		if ps.Mid == nil || *ps.Mid != 7 || ps.MatchKey != wantKey || ps.SiteId != "src_lz" {
			t.Fatalf("master ps: %+v", ps)
		}
		if ps.SourceVodID != d.VodID { // 去重统计依赖源站影片ID落在每行
			t.Fatalf("master SourceVodID = %d, want %d", ps.SourceVodID, d.VodID)
		}
	}
	// slave: dbid>0 → 额外按 hash(dbid) 再存一份 → 2 组 × 2 key = 4 行, mid 为 nil
	slave := buildPlaySources(d, "src_sn", nil)
	if len(slave) != 4 {
		t.Fatalf("slave sources = %d, want 4", len(slave))
	}
	for _, ps := range slave {
		if ps.Mid != nil {
			t.Fatalf("slave should have nil mid: %+v", ps)
		}
		// 双键 4 行同属一部片, 共享同一 source_vod_id → CountBySite 可去重为 1 片
		if ps.SourceVodID != d.VodID {
			t.Fatalf("slave SourceVodID = %d, want %d", ps.SourceVodID, d.VodID)
		}
	}
}

func TestBuildPlaySources_SkipExplainer(t *testing.T) {
	d := sampleDetail()
	d.TypeName = "电影解说"
	if got := buildPlaySources(d, "src_sn", nil); got != nil {
		t.Fatalf("解说类应跳过, got %d rows", len(got))
	}
	// 主站不跳过(按 mid)
	mid := int64(7)
	if got := buildPlaySources(d, "src_lz", &mid); len(got) == 0 {
		t.Fatal("master 解说 不应被跳过")
	}
}

func TestParseCategories(t *testing.T) {
	cl := []filmClass{{TypeID: 1, TypePid: 0, TypeName: "电影"}, {TypeID: 11, TypePid: 1, TypeName: "动作"}}
	cats := parseCategories(cl)
	if len(cats) != 2 || !cats[0].Show || cats[1].Pid != 1 || cats[1].Name != "动作" {
		t.Fatalf("parse categories: %+v", cats)
	}
}

// jsonEpisodeCount 取最长线路集数(供搜索预览展示)。
func TestJSONEpisodeCount(t *testing.T) {
	d := filmDetail{
		VodPlayFrom: "snm3u8$$$bfm3u8",
		VodPlayNote: "$$$",
		// 线路1: 2 集; 线路2: 3 集 → 取 3
		VodPlayURL: "第1集$a/1.m3u8#第2集$a/2.m3u8$$$第1集$b/1.m3u8#第2集$b/2.m3u8#第3集$b/3.m3u8",
	}
	if n := jsonEpisodeCount(d); n != 3 {
		t.Fatalf("jsonEpisodeCount=%d want 3(最长线路)", n)
	}
	if n := jsonEpisodeCount(filmDetail{}); n != 0 {
		t.Fatalf("空片源应为 0, got %d", n)
	}
}
