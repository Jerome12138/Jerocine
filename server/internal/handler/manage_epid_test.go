package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"server/internal/service"
)

// T4 单测（epid 源头校正层 · 管理端入口，2026-11-12）。
//
// handler 层没有 DB 基建 ⇒ 只测**不需要装配派生服务/台账**就能确定的行为：
//   - D3 生效开关把守：apply / recompute 关着 ⇒ 403 problem+json（且不触达服务层）；
//   - 请求体绑定：import 非 JSON / 非数组 ⇒ 400；enabled 缺字段 ⇒ 400；apply 空 body（省略 targets）
//     放行到服务层（缺省=全部 active 语义），不是 400；
//   - 开关开着但派生服务未装配 ⇒ 显式 500 problem+json（fail-closed，绝不 panic）。
//
// 台账读写路径（ListAll / UpsertPending / ApplyOverrides 的 SQL 行为）在 service/repo 层用 sqlmock 测；
// 401 由 router.go mg 组中间件（AuthToken + RequireAdmin）保证，不在 handler 单测范围。
func TestManageEpidOverride_switchAndBinding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &Handlers{} // 被测 handler 不依赖 Handlers 的任何字段
	r.POST("/import", h.ManageEpidOverrideImport)
	r.POST("/apply", h.ManageEpidOverrideApply)
	r.POST("/recompute", h.ManageEpidOverrideRecompute)
	r.GET("/overrides", h.ManageEpidOverrideList)
	r.PUT("/enabled", h.ManageEpidOverrideSetEnabled)
	t.Cleanup(func() { service.SetEpidOverrideEnabled(false) }) // 开关是 service 包级状态，测毕还原默认关

	send := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	// ① 开关默认关（D3）：apply / recompute ⇒ 403 problem+json
	if w := send(http.MethodPost, "/apply", `{"targets":[{"srcRowId":9,"epIndex":2}]}`); w.Code != http.StatusForbidden {
		t.Fatalf("开关关着 apply 应 403, got %d: %s", w.Code, w.Body.String())
	}
	if w := send(http.MethodPost, "/apply", `{"targets":[]}`); !strings.Contains(w.Header().Get("Content-Type"), "problem+json") {
		t.Fatalf("403 应为 problem+json, got %q", w.Header().Get("Content-Type"))
	}
	if w := send(http.MethodPost, "/recompute", `{"targets":[{"srcRowId":9,"epIndex":2}]}`); w.Code != http.StatusForbidden {
		t.Fatalf("开关关着 recompute 应 403, got %d: %s", w.Code, w.Body.String())
	}

	// ② import 不受开关影响（录入先行）：非法 body 在绑定层拒绝
	if w := send(http.MethodPost, "/import", `not-json`); w.Code != http.StatusBadRequest {
		t.Fatalf("import 非 JSON 应 400, got %d: %s", w.Code, w.Body.String())
	}
	if w := send(http.MethodPost, "/import", `{"srcRowId":9}`); w.Code != http.StatusBadRequest {
		t.Fatalf("import 传对象应 400（body 必须是候选行 JSON 数组）, got %d: %s", w.Code, w.Body.String())
	}

	// ③ enabled 开关本体：缺字段 ⇒ 400；合法 ⇒ 200 并真正翻转开关
	if w := send(http.MethodPut, "/enabled", `{}`); w.Code != http.StatusBadRequest {
		t.Fatalf("enabled 缺字段应 400, got %d: %s", w.Code, w.Body.String())
	}
	if w := send(http.MethodPut, "/enabled", `{"enabled":true}`); w.Code != http.StatusOK {
		t.Fatalf("enabled 合法应 200, got %d: %s", w.Code, w.Body.String())
	}
	if !service.EpidOverrideEnabled() {
		t.Fatalf("PUT {\"enabled\":true} 后 service.EpidOverrideEnabled() 应为 true")
	}

	// ④ 开关开着但派生服务未装配：apply 空 body（缺省=全部 active）放行到服务层 ⇒ 显式 500（非 400）
	if w := send(http.MethodPost, "/apply", ``); w.Code != http.StatusInternalServerError {
		t.Fatalf("未装配派生服务 apply 应 500 problem+json, got %d: %s", w.Code, w.Body.String())
	}
	if w := send(http.MethodPost, "/apply", `{"targets":[{"srcRowId":9}]}`); !strings.Contains(w.Header().Get("Content-Type"), "problem+json") {
		t.Fatalf("500 应为 problem+json, got %q", w.Header().Get("Content-Type"))
	}

	// ⑤ GET 列表不受开关影响；未装配 ⇒ 500 problem+json（只读路径同样 fail-closed）
	if w := send(http.MethodGet, "/overrides", ``); w.Code != http.StatusInternalServerError {
		t.Fatalf("未装配 GET overrides 应 500, got %d: %s", w.Code, w.Body.String())
	}
}
