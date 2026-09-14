package douban

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient 把客户端指向一个本地假豆瓣; 限速归零, 让测试不必真等 5 秒。
func newTestClient(t *testing.T, h http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New()
	c.SetBaseURL(srv.URL)
	c.SetInterval(0)
	return c, srv
}

// itemsBody 造一份合法响应体。
func itemsBody(items ...Item) string {
	b, _ := json.Marshal(map[string]any{"subject_collection_items": items})
	return string(b)
}

func item(id int64, title string) Item {
	return Item{Id: id, Title: title, Year: "2024"}
}

// TestFetch_PaginationStopsAtShortPage 不满一页即到底: 50 + 3 → 2 页 53 条, 不再请求 start=100。
func TestFetch_PaginationStopsAtShortPage(t *testing.T) {
	var calls int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		switch r.URL.Query().Get("start") {
		case "0":
			items := make([]Item, 0, pageSize)
			for i := 0; i < pageSize; i++ {
				items = append(items, item(int64(i+1), fmt.Sprintf("片%d", i+1)))
			}
			_, _ = w.Write([]byte(itemsBody(items...)))
		case "50":
			_, _ = w.Write([]byte(itemsBody(item(51, "甲"), item(52, "乙"), item(53, "丙"))))
		default:
			t.Errorf("不该请求 start=%s", r.URL.Query().Get("start"))
		}
	})

	cols := []Collection{{Name: "movie_hot_gaia", Label: "热门电影", Depth: 308}}
	got, err := c.Fetch(context.Background(), cols)
	if err != nil {
		t.Fatalf("Fetch err: %v", err)
	}
	if len(got) != 53 {
		t.Fatalf("条目数=%d, want 53", len(got))
	}
	if got[0].Rank != 1 || got[52].Rank != 53 {
		t.Fatalf("位次不连续: first=%d last=%d", got[0].Rank, got[52].Rank)
	}
	if got[52].Title != "丙" || got[52].Depth != 308 || got[52].Collection != "movie_hot_gaia" {
		t.Fatalf("条目携带信息不全: %+v", got[52])
	}
	st := c.Stats()
	if st.Pages != 2 || st.Items != 53 || st.Empty != 0 || st.Skipped != 0 {
		t.Fatalf("stats=%+v", st)
	}
	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Fatalf("请求次数=%d, want 2", n)
	}
}

// TestFetch_EmptyFirstPageIsNotBottom 首页空返回 = 限流/异常, 只记 Empty, 绝不能解释成"榜到底了"。
func TestFetch_EmptyFirstPageIsNotBottom(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(itemsBody())) // 合法 JSON, 但一条都没有
	})

	got, err := c.Fetch(context.Background(), Collections[:1])
	if err != nil {
		t.Fatalf("Fetch err: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("空返回不该产出条目: %+v", got)
	}
	if st := c.Stats(); st.Empty != 1 || st.Pages != 0 {
		t.Fatalf("stats=%+v (Empty 必须记 1, 供调用方判本轮不可信)", st)
	}
}

// TestFetch_MissingItemsKeyRetriesThenSkips 结构异常(缺 subject_collection_items) → 重试一次后跳过该集合,
// 但**不影响其他集合** —— 一个集合抖动不能让整轮退化成兜底分。
func TestFetch_MissingItemsKeyRetriesThenSkips(t *testing.T) {
	var badCalls int32
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/broken") {
			atomic.AddInt32(&badCalls, 1)
			_, _ = w.Write([]byte(`{"count":0,"start":0}`)) // 少 items 字段
			return
		}
		_, _ = w.Write([]byte(itemsBody(item(1, "好片"))))
	})

	cols := []Collection{
		{Name: "broken", Label: "坏榜", Depth: 10},
		{Name: "good", Label: "好榜", Depth: 10},
	}
	got, err := c.Fetch(context.Background(), cols)
	if err != nil {
		t.Fatalf("Fetch err: %v", err)
	}
	if len(got) != 1 || got[0].Title != "好片" {
		t.Fatalf("好榜应照常产出: %+v", got)
	}
	if n := atomic.LoadInt32(&badCalls); n != maxAttempts {
		t.Fatalf("坏榜请求次数=%d, want %d(重试一次)", n, maxAttempts)
	}
	if st := c.Stats(); st.Skipped != 1 || st.Items != 1 {
		t.Fatalf("stats=%+v", st)
	}
}

// TestFetch_HTTPErrorSkipped 非 200 同样只跳过, 不污染其他集合。
func TestFetch_HTTPErrorSkipped(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	got, err := c.Fetch(context.Background(), Collections[:1])
	if err != nil {
		t.Fatalf("Fetch 不该整体报错: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("403 不该产出条目: %+v", got)
	}
	if st := c.Stats(); st.Skipped != 1 {
		t.Fatalf("stats=%+v", st)
	}
}

// TestFetch_ContextCancelStops 取消后不再继续打对端(采集/刷新被中断时立刻收手)。
func TestFetch_ContextCancelStops(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(itemsBody(item(1, "片"))))
	})
	got, _ := c.Fetch(ctx, Collections)
	if len(got) != 0 {
		t.Fatalf("已取消仍抓到条目: %+v", got)
	}
}

// TestClient_RateLimitInterval 相邻两次请求的间隔不小于设定值(IP 被封的红线)。
func TestClient_RateLimitInterval(t *testing.T) {
	const gap = 120 * time.Millisecond
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(itemsBody(item(1, "片"))))
	})
	c.SetInterval(gap)

	start := time.Now()
	if _, err := c.Fetch(context.Background(), []Collection{
		{Name: "a", Label: "A", Depth: 1},
		{Name: "b", Label: "B", Depth: 1},
	}); err != nil {
		t.Fatalf("Fetch err: %v", err)
	}
	if elapsed := time.Since(start); elapsed < gap {
		t.Fatalf("两次集合抓取仅耗时 %v, 小于限速间隔 %v", elapsed, gap)
	}
}

// TestItem_FlexibleNumericFields 线上实测(2026-09-12): subject_collection_items 的 id
// 是**字符串**(`"id": "35423605"`), 强类型 int64 让整页 unmarshal 失败、首轮部署整轮作废。
// id / rating.count / rating.value 都必须同时接受 number 与 string, 单字段异常损失该字段而非整页。
func TestItem_FlexibleNumericFields(t *testing.T) {
	body := `{"subject_collection_items":[
		{"id":"35423605","title":"字符串id","year":"2024",
		 "rating":{"value":"8.6","count":"12345"}},
		{"id":123,"title":"数字id","year":"2023","rating":{"value":9.1,"count":456}},
		{"id":null,"title":"空id","year":"2022","rating":null},
		{"id":"not-a-number","title":"垃圾id","year":"2021"}
	]}`
	var resp struct {
		Items []Item `json:"subject_collection_items"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("宽松解析后不该再因字段形态报错: %v", err)
	}
	got := resp.Items
	if len(got) != 4 {
		t.Fatalf("条目数=%d, want 4", len(got))
	}
	if got[0].Id != 35423605 || got[0].Rating.Value != 8.6 || got[0].Rating.Count != 12345 {
		t.Fatalf("字符串形态解析失败: %+v", got[0])
	}
	if got[1].Id != 123 || got[1].Rating.Value != 9.1 || got[1].Rating.Count != 456 {
		t.Fatalf("数字形态解析失败: %+v", got[1])
	}
	if got[2].Id != 0 || got[2].Rating.Value != 0 {
		t.Fatalf("null 应按缺省 0: %+v", got[2])
	}
	if got[3].Id != 0 || got[3].Title != "垃圾id" {
		t.Fatalf("垃圾值只损失该字段: %+v", got[3])
	}
}

func TestItem_YearIntAndNames(t *testing.T) {
	it := Item{Year: "2024", Directors: json.RawMessage(`[{"name":"导演甲"}]`),
		Actors: json.RawMessage(`["演员乙","演员丙"]`)}
	if it.YearInt() != 2024 {
		t.Fatalf("YearInt=%d", it.YearInt())
	}
	names := it.Names()
	if len(names) != 3 || names[0] != "导演甲" || names[2] != "演员丙" {
		t.Fatalf("Names=%v", names)
	}
	if (Item{Year: "未知"}).YearInt() != 0 {
		t.Fatal("非法年份应为 0")
	}
}

// TestHotBoardLabel 榜单名兜底: 缺 hot_board 时要按分类补全(用户要求"显示全, 别只显示热门"),
// 但不在榜 / 分类未知时不能硬造一个榜单名。
func TestHotBoardLabel(t *testing.T) {
	cases := []struct {
		name string
		pid  int64
		raw  string
		rank int
		want string
	}{
		{"落库的集合名照常翻译", 1, "movie_hot_gaia", 3, "热门电影"},
		{"动漫片的分类热榜", 4, "tv_animation", 1, "热门动漫"},
		{"缺榜名 → 按分类兜底出全称", 4, "", 1, "热门动漫"},
		{"缺榜名 → 剧集兜底", 2, "", 7, "热门剧集"},
		{"缺榜名 → 综艺兜底", 3, "", 2, "热门综艺"},
		{"缺榜名 → 电影兜底", 1, "", 1, "热门电影"},
		{"不在榜就不造榜名", 4, "", 0, ""},
		{"未知分类不硬造", 99, "", 5, ""},
		{"历史集合名保留原名(不展示成空白)", 1, "legacy_board", 2, "legacy_board"},
	}
	for _, c := range cases {
		if got := HotBoardLabel(c.pid, c.raw, c.rank); got != c.want {
			t.Errorf("%s: HotBoardLabel(%d,%q,%d)=%q, want %q",
				c.name, c.pid, c.raw, c.rank, got, c.want)
		}
	}
}
