package router

import (
	"testing"

	"github.com/gin-gonic/gin"

	"server/internal/config"
	"server/internal/handler"
)

// TestRegisterNoConflict 验证全部路由能成功注册(若 /films/classify 与 /films/:mid 等冲突, gin 会 panic)。
func TestRegisterNoConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("route registration panicked: %v", rec)
		}
	}()
	Register(r, &handler.Handlers{}, nil, &config.Config{})

	want := map[string]bool{
		"GET /api/v1/home":              false,
		"GET /api/v1/films/classify":    false,
		"GET /api/v1/films/:mid":        false,
		"GET /api/v1/films/:mid/play":   false,
		"POST /api/v1/auth/login":       false,
		"DELETE /api/v1/me/histories":   false,
		"POST /api/v1/manage/spider/reset": false,
		// 全量测速静态段与单源 :id 测速参数段同级共存, 不得冲突
		"POST /api/v1/manage/collect-sources/test":     false,
		"POST /api/v1/manage/collect-sources/:id/test": false,
		// 健康度面板静态段与 GET :id 同级共存
		"GET /api/v1/manage/collect-sources/health": false,
		"GET /api/v1/manage/collect-sources/:id":    false,
		// 弹幕: 路径 A 整集聚合静态段与路径 B :provider 参数段同级共存, 不得冲突
		"GET /api/v1/danmu/bilibili":              false,
		"GET /api/v1/danmu/:provider/episodes":   false,
		"GET /api/v1/danmu/:provider/segments":   false,
		"GET /api/v1/danmu/:provider/comments":   false,
	}
	for _, ri := range r.Routes() {
		key := ri.Method + " " + ri.Path
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for k, seen := range want {
		if !seen {
			t.Errorf("route not registered: %s", k)
		}
	}
}
