package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"server/internal/service"
)

func TestM3u8StreamRejectsUnsignedRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := &Handlers{M3u8: service.NewM3u8Service([]byte("test-signing-key"))}
	r := gin.New()
	r.GET("/m3u8/stream", h.M3u8Stream)

	req := httptest.NewRequest(http.MethodGet, "/m3u8/stream?src=https://cdn.example/seg.ts", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status=%d want %d", w.Code, http.StatusForbidden)
	}
}
