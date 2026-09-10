package mysql

import (
	"reflect"
	"strings"
	"testing"

	"server/internal/domain/entity"
)

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
