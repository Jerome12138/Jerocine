package mysql

import (
	"reflect"
	"strings"
	"testing"

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
