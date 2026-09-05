// Package dto 定义 API 请求/响应体与统一响应助手。
// 契约: 成功直返 data(语义化 HTTP status); 失败统一 RFC7807 application/problem+json。
package dto

import (
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"

	"server/internal/domain"
)

// Problem RFC7807 错误体。
type Problem struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	TraceId string `json:"traceId"`
	Details any    `json:"details,omitempty"`
}

// PageMeta 分页元信息。
type PageMeta struct {
	Current   int   `json:"current"`
	Size      int   `json:"size"`
	Total     int64 `json:"total"`
	PageCount int64 `json:"pageCount"`
}

// Paginated 分页响应体。
type Paginated struct {
	List any      `json:"list"`
	Page PageMeta `json:"page"`
}

const traceKey = "traceId"

// OK 200 直返 data。
func OK(c *gin.Context, data any) { c.JSON(http.StatusOK, data) }

// Created 201。
func Created(c *gin.Context, data any) { c.JSON(http.StatusCreated, data) }

// Accepted 202(异步任务已受理)。
func Accepted(c *gin.Context, data any) { c.JSON(http.StatusAccepted, data) }

// NoContent 204。
func NoContent(c *gin.Context) { c.Status(http.StatusNoContent) }

// Page 组装分页响应(list 保证非 nil → 序列化为 [])。
func Page(c *gin.Context, list any, current, size int, total int64) {
	if list == nil {
		list = []any{}
	}
	pc := int64(0)
	if size > 0 {
		pc = (total + int64(size) - 1) / int64(size)
	}
	c.JSON(http.StatusOK, Paginated{List: list, Page: PageMeta{Current: current, Size: size, Total: total, PageCount: pc}})
}

// Fail 把领域错误映射为 HTTP 状态 + problem+json。
func Fail(c *gin.Context, err error) {
	status := mapStatus(err)
	Error(c, status, err.Error())
}

// Error 写出指定状态的 problem+json。
func Error(c *gin.Context, status int, msg string) {
	c.Header("Content-Type", "application/problem+json")
	c.AbortWithStatusJSON(status, Problem{Code: status, Message: msg, TraceId: TraceId(c)})
}

func mapStatus(err error) int {
	switch err {
	case domain.ErrNotFound, domain.ErrMovieNotFound, domain.ErrUserNotFound:
		return http.StatusNotFound
	case domain.ErrInvalidArgument:
		return http.StatusBadRequest
	case domain.ErrConflict:
		return http.StatusConflict
	case domain.ErrUnauthorized:
		return http.StatusUnauthorized
	case domain.ErrForbidden:
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

// WriteCached 公开读接口: 计算 ETag, 命中 If-None-Match 返 304, 否则带 s-maxage/SWR 缓存头返 data。
// 让 nginx/openserst/CDN 在边缘吃掉跨国 RTT。
func WriteCached(c *gin.Context, data any, sMaxAge int) {
	b, err := json.Marshal(data)
	if err != nil {
		Error(c, http.StatusInternalServerError, "marshal error")
		return
	}
	sum := sha1.Sum(b)
	etag := `"` + hex.EncodeToString(sum[:]) + `"`
	c.Header("ETag", etag)
	c.Header("Cache-Control", "public, s-maxage="+itoa(sMaxAge)+", stale-while-revalidate="+itoa(sMaxAge*2))
	if match := c.GetHeader("If-None-Match"); match == etag {
		c.Status(http.StatusNotModified)
		return
	}
	c.Data(http.StatusOK, "application/json; charset=utf-8", b)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// TraceId 取/生成当前请求 trace id。
func TraceId(c *gin.Context) string {
	if v, ok := c.Get(traceKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// NewTraceId 生成 16 字节 hex trace id。
func NewTraceId() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// SetTraceId 中间件用: 写入 ctx + 响应头。
func SetTraceId(c *gin.Context) string {
	id := NewTraceId()
	c.Set(traceKey, id)
	c.Header("X-Trace-Id", id)
	return id
}
