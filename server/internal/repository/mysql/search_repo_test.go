package mysql

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// TestFallbackLast 兜底项(其它/其他)必须固定在选项列表末尾, 其余相对顺序不变。
func TestFallbackLast(t *testing.T) {
	t.Run("其它在中间→移到末尾", func(t *testing.T) {
		in := []repository.TagOption{{Name: "美国"}, {Name: "大陆"}, {Name: "其它"}, {Name: "日本"}}
		got := fallbackLast(in)
		want := []string{"美国", "大陆", "日本", "其它"}
		if names(got) != strings.Join(want, ",") {
			t.Fatalf("got %v, want %v", names(got), want)
		}
	})
	t.Run("其他同样处理", func(t *testing.T) {
		in := []repository.TagOption{{Name: "英语"}, {Name: "其他"}, {Name: "日语"}}
		got := fallbackLast(in)
		want := []string{"英语", "日语", "其他"}
		if names(got) != strings.Join(want, ",") {
			t.Fatalf("got %v, want %v", names(got), want)
		}
	})
	t.Run("无兜底项原样返回", func(t *testing.T) {
		in := []repository.TagOption{{Name: "剧情"}, {Name: "喜剧"}}
		if got := fallbackLast(in); names(got) != "剧情,喜剧" {
			t.Fatalf("got %v", names(got))
		}
	})
	t.Run("多个兜底项全部移到最后且保持相对顺序", func(t *testing.T) {
		in := []repository.TagOption{{Name: "其它"}, {Name: "动作"}, {Name: "其他"}, {Name: "爱情"}}
		got := fallbackLast(in)
		want := []string{"动作", "爱情", "其它", "其他"}
		if names(got) != strings.Join(want, ",") {
			t.Fatalf("got %v, want %v", names(got), want)
		}
	})
	t.Run("空列表", func(t *testing.T) {
		if got := fallbackLast(nil); len(got) != 0 {
			t.Fatalf("got %v, want empty", got)
		}
	})
}

func names(opts []repository.TagOption) string {
	ns := make([]string, 0, len(opts))
	for _, o := range opts {
		ns = append(ns, o.Name)
	}
	return strings.Join(ns, ",")
}

// TestSearchUpsertColsCoverEntity 与 TestMovieUpsertColsCoverEntity 同因:
// 保证采集回写读模型时不会误刷 created_at / deleted_at, 也不会漏掉新增字段。
func TestSearchUpsertColsCoverEntity(t *testing.T) {
	inList := make(map[string]bool, len(searchUpsertCols))
	for _, c := range searchUpsertCols {
		if inList[c] {
			t.Fatalf("searchUpsertCols 有重复列: %s", c)
		}
		inList[c] = true
	}
	typ := reflect.TypeOf(entity.MovieSearch{})
	for i := 0; i < typ.NumField(); i++ {
		col := strings.TrimPrefix(strings.Split(typ.Field(i).Tag.Get("gorm"), ";")[0], "column:")
		if col == "" {
			t.Fatalf("entity.MovieSearch 字段 %s 缺少 gorm column 标签", typ.Field(i).Name)
		}
		want := !searchUpsertExclude[col]
		if got := inList[col]; got != want {
			t.Fatalf("列 %s 在 searchUpsertCols 中=%v, 期望=%v (请同步 searchUpsertCols / searchUpsertExclude)", col, got, want)
		}
	}
}

// TestHotBoardOrder 热榜排序键的 SQL 组装回归保护(方案 §1.1):
// 单榜默认按该榜位次(枪版片位次后移)/ 全榜默认按热度(枪版动态减一个步长+多榜回血)/
// score/latest 各分支 / rank 在全榜回落 hot / 未识别 sort(如 recent)同样回落默认。
func TestHotBoardOrder(t *testing.T) {
	tc := hotBoardTCExpr

	t.Run("单榜默认=按榜位+枪版位次后移", func(t *testing.T) {
		got := hotBoardOrder("movie_hot_gaia", "")
		if !strings.Contains(got, "h.`rank`") {
			t.Fatalf("单榜默认排序缺少榜位列: %s", got)
		}
		if strings.Count(got, tc) != 1 {
			t.Fatalf("单榜默认排序应恰好引用 1 次枪版表达式, got=%q", got)
		}
		if !strings.Contains(got, strconv.Itoa(domain.HotBoardTCRankDrop)+") ASC") {
			t.Fatalf("单榜默认排序应含枪版位次后移 %d, got=%q", domain.HotBoardTCRankDrop, got)
		}
		if got != hotBoardOrder("movie_hot_gaia", "rank") {
			t.Fatalf("显式 rank 应与默认一致")
		}
	})

	t.Run("单榜 hot/score/latest 走各自键且不含枪版表达式", func(t *testing.T) {
		if got := hotBoardOrder("movie_hot_gaia", "hot"); !strings.Contains(got, "m.hot_score DESC") || strings.Contains(got, tc) {
			t.Fatalf("单榜 hot 排序异常: %q", got)
		}
		if got := hotBoardOrder("movie_hot_gaia", "score"); !strings.Contains(got, "m.db_score DESC") || strings.Contains(got, tc) {
			t.Fatalf("单榜 score 排序异常: %q", got)
		}
		if got := hotBoardOrder("movie_hot_gaia", "latest"); !strings.Contains(got, "m.pub_date DESC") || strings.Contains(got, tc) {
			t.Fatalf("单榜 latest 排序异常: %q", got)
		}
	})

	t.Run("全榜默认=热度+枪版降级+多榜回血", func(t *testing.T) {
		got := hotBoardOrder("", "")
		if !strings.Contains(got, "m.hot_score - ") {
			t.Fatalf("全榜默认排序缺少热度基: %s", got)
		}
		if strings.Count(got, tc) != 2 {
			t.Fatalf("全榜默认排序应引用 2 次枪版表达式(降级+回血), got=%q", got)
		}
		if !strings.Contains(got, strconv.Itoa(domain.HotBoardTCStepDrop)) {
			t.Fatalf("全榜默认排序应含枪版降级步长 %d, got=%q", domain.HotBoardTCStepDrop, got)
		}
		wantBonus := "LEAST(GREATEST(COALESCE(b.bc,1)-1,0)*" + strconv.Itoa(domain.HotBoardTCMultiBoardBonus) +
			"," + strconv.Itoa(domain.HotBoardTCMultiBoardCap) + ")"
		if !strings.Contains(got, wantBonus) {
			t.Fatalf("全榜默认排序应含多榜回血 %s, got=%q", wantBonus, got)
		}
	})

	t.Run("全榜 rank/recent 回落热度默认", func(t *testing.T) {
		want := hotBoardOrder("", "")
		if got := hotBoardOrder("", "rank"); got != want {
			t.Fatalf("全榜 rank 应回落热度默认")
		}
		if got := hotBoardOrder("", "recent"); got != want {
			t.Fatalf("全榜 recent(未实现)应回落热度默认, got=%q", got)
		}
		if got := hotBoardOrder("movie_hot_gaia", "recent"); got != hotBoardOrder("movie_hot_gaia", "") {
			t.Fatalf("单榜 recent(未实现)应回落榜位默认, got=%q", got)
		}
	})
}
