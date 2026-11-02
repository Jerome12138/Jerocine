package handler

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestQueryInt64s 分类多选参数 `catIds=1,2,3` 的解析:
// 常规串按逗号拆; 非法段(非数字/0/负数)跳过不报错; 空串/缺失返回 nil。
func TestQueryInt64s(t *testing.T) {
	gin.SetMode(gin.TestMode)
	newCtx := func(query string) *gin.Context {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodGet, "/?"+query, nil)
		return c
	}

	if got := queryInt64s(newCtx("catIds=5,9,3"), "catIds"); !reflect.DeepEqual(got, []int64{5, 9, 3}) {
		t.Fatalf("常规串应逐段解析, got %v", got)
	}
	// 带空格(%20) + 混入非法段: 非法段(abc/0/-2)跳过, 合法段保留
	if got := queryInt64s(newCtx("catIds=%205%20,%20abc,%200,%20-2,%209"), "catIds"); !reflect.DeepEqual(got, []int64{5, 9}) {
		t.Fatalf("非法段应跳过不报错, got %v", got)
	}
	// 全非法 → nil(当没传)
	if got := queryInt64s(newCtx("catIds=abc,0"), "catIds"); got != nil {
		t.Fatalf("全非法段应返回 nil, got %v", got)
	}
	// 缺参 / 空串 → nil
	if got := queryInt64s(newCtx(""), "catIds"); got != nil {
		t.Fatalf("缺参应返回 nil, got %v", got)
	}
	if got := queryInt64s(newCtx("catIds="), "catIds"); got != nil {
		t.Fatalf("空串应返回 nil, got %v", got)
	}
}
