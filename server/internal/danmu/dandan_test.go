package danmu

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDandanClientTokenPathPrefix(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"animes":[]}`))
	}))
	defer srv.Close()

	c := NewDandan(srv.URL, "sec-token")
	if !c.Enabled() {
		t.Fatal("Enabled() = false, want true")
	}
	if _, err := c.SearchAnime(context.Background(), "庆余年"); err != nil {
		t.Fatalf("SearchAnime: %v", err)
	}
	if gotPath != "/sec-token/api/v2/search/anime" {
		t.Fatalf("path = %q, want token as path prefix", gotPath)
	}
	_ = gotAuth // 该协议鉴权走路径, 不应依赖请求头
}

func TestDandanClientDisabledWhenBlank(t *testing.T) {
	if (&DandanClient{}).Enabled() {
		t.Fatal("blank client should be disabled")
	}
	if NewDandan("http://x", "").Enabled() {
		t.Fatal("missing token should be disabled")
	}
}

func TestSearchAndBangumiParse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/tok/api/v2/search/anime", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("keyword") != "庆余年" {
			w.WriteHeader(400)
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"animes":[{"animeId":2125916,"animeTitle":"庆余年 第二季(2024)【国产剧】from tencent","episodeCount":37}]}`))
	})
	mux.HandleFunc("/tok/api/v2/bangumi/2125916", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"bangumi":{"episodes":[{"episodeId":10002,"episodeTitle":"第1集","episodeNumber":"1"},{"episodeId":10004,"episodeTitle":"第3集","episodeNumber":"3"}]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewDandan(srv.URL, "tok")
	animes, err := c.SearchAnime(context.Background(), "庆余年")
	if err != nil || len(animes) != 1 || animes[0].AnimeID != 2125916 {
		t.Fatalf("search: %v %+v", err, animes)
	}
	eps, err := c.Bangumi(context.Background(), 2125916)
	if err != nil || len(eps) != 2 || eps[1].EpisodeID != 10004 {
		t.Fatalf("bangumi: %v %+v", err, eps)
	}
}

func TestPickAnimeScoring(t *testing.T) {
	animes := []DandanAnime{
		{AnimeID: 1, AnimeTitle: "庆余年 第一季(2019)【国产剧】from tencent", EpisodeCount: 46},
		{AnimeID: 2, AnimeTitle: "庆余年 第二季(2024)【国产剧】from tencent", EpisodeCount: 37},
		{AnimeID: 3, AnimeTitle: "完全无关的剧", EpisodeCount: 12},
	}
	got := PickAnime(animes, "庆余年", 2024, 3)
	if got == nil || got.AnimeID != 2 {
		t.Fatalf("PickAnime year match: %+v", got)
	}
	// 无年份时: 两候选都含关键词且集数够 → 取靠前且集数足够的（稳定序）
	got = PickAnime(animes, "庆余年", 0, 3)
	if got == nil || got.AnimeID != 1 {
		t.Fatalf("PickAnime fallback first: %+v", got)
	}
	// 集数超出的候选降分
	long := []DandanAnime{{AnimeID: 9, AnimeTitle: "某剧(2024)", EpisodeCount: 2}}
	if p := PickAnime(long, "某剧", 2024, 30); p == nil {
		t.Fatal("集数超出也应保留候选(仅降分), 不应返回 nil")
	}
	if PickAnime(animes, "不存在", 0, 1) != nil {
		t.Fatal("无关键词命中应返回 nil")
	}
	if PickAnime(animes, "", 0, 1) != nil {
		t.Fatal("空关键词应返回 nil")
	}
}

func TestPickEpisodeOrder(t *testing.T) {
	eps := []DandanEpisode{
		{EpisodeID: 11, EpisodeTitle: "第01集 开端", EpisodeNumber: "1"},
		{EpisodeID: 12, EpisodeTitle: "第02集", EpisodeNumber: "2"},
		{EpisodeID: 13, EpisodeTitle: "特别篇", EpisodeNumber: ""},
	}
	// 1) episodeNumber 精确命中
	if e := PickEpisode(eps, 2); e == nil || e.EpisodeID != 12 {
		t.Fatalf("episodeNumber 命中: %+v", e)
	}
	// 2) episodeNumber 缺失时按标题「第N集」；季号不是集号（第2季 ≠ 第2集）
	if e := PickEpisode([]DandanEpisode{{EpisodeID: 31, EpisodeTitle: "第2季"}}, 2); e != nil {
		t.Fatalf("季号不应命中集号: %+v", e)
	}
	titleOnly := []DandanEpisode{{EpisodeID: 21, EpisodeTitle: "第3期 上"}, {EpisodeID: 22, EpisodeTitle: "番外"}}
	if e := PickEpisode(titleOnly, 3); e == nil || e.EpisodeID != 21 {
		t.Fatalf("标题命中: %+v", e)
	}
	// 3) 都没有 → 按下标
	if e := PickEpisode(titleOnly, 2); e == nil || e.EpisodeID != 22 {
		t.Fatalf("下标回退: %+v", e)
	}
	if PickEpisode(eps, 99) != nil {
		t.Fatal("超界应返回 nil")
	}
	if PickEpisode(nil, 1) != nil || PickEpisode(eps, 0) != nil {
		t.Fatal("空列表/非法集号应返回 nil")
	}
}

func TestParseDandanComments(t *testing.T) {
	raw := []DandanRawComment{
		{P: "12.5,1,16777215,[qq]", M: "滚动白"},
		{P: "60,4,16711680,[qq]", M: "底部红"},
		{P: "61,5,255,[qq]", M: "顶部蓝"},
		{P: "bad,1,1,[qq]", M: "时间坏"},
		{P: "62,1,999,[qq]", M: ""}, // 空文本跳过
	}
	out := ParseDandanComments(raw)
	if len(out) != 3 {
		t.Fatalf("len = %d, want 3", len(out))
	}
	if out[0].Time != 12500 || out[0].Mode != 1 || out[0].Color != 0xFFFFFF {
		t.Fatalf("第一条映射: %+v", out[0])
	}
	if out[1].Mode != 4 || out[1].Color != 0xFF0000 || out[2].Mode != 5 || out[2].Color != 255 {
		t.Fatalf("模式/颜色映射: %+v %+v", out[1], out[2])
	}
}

func TestDandanCommentSuccessFalseWithData(t *testing.T) {
	// 实测: 上游对缓存命中的整集可能带 success:false 但 comments 完整 —— 以数据为准
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"count":1,"comments":[{"cid":1,"p":"1.5,1,255,[qq]","m":"a"}],"success":false,"errorMessage":""}`))
	}))
	defer srv.Close()
	c := NewDandan(srv.URL, "t")
	out, err := c.Comments(context.Background(), 1)
	if err != nil || len(out) != 1 || out[0].M != "a" {
		t.Fatalf("success=false 有数据应返回: %v %+v", err, out)
	}
}

func TestDandanCommentErrorResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":false,"errorMessage":"boom"}`))
	}))
	defer srv.Close()
	c := NewDandan(srv.URL, "t")
	if _, err := c.Comments(context.Background(), 1); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("success=false 应报错: %v", err)
	}
	var raw json.RawMessage = []byte(`{}`)
	_ = raw
}
