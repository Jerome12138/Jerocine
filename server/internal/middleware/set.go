// Package middleware 提供 gin 中间件: trace / recovery / cors / 鉴权 / 限流 / 缓存头。
package middleware

import (
	"crypto/subtle"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"server/internal/cache"
	"server/internal/dto"
	"server/internal/platform/reqctx"
	"server/internal/service"
)

// ctx keys
const (
	CtxUserID = "userID" // uint
	CtxRole   = "role"   // int
	CtxToken  = "token"  // string
)

// Trace 为每个请求生成 trace id(写 ctx + X-Trace-Id 头), 并把客户端 IP/UA 放进 request context
// (登录会话元数据要用, 见 platform/reqctx)。
func Trace() gin.HandlerFunc {
	return func(c *gin.Context) {
		dto.SetTraceId(c)
		c.Request = c.Request.WithContext(
			reqctx.With(c.Request.Context(), c.ClientIP(), c.Request.UserAgent()))
		c.Next()
	}
}

// Recovery panic 兜底, 返回 500 problem(独立拆出, 不再混在 cors)。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("panic %s %s: %v", c.Request.Method, c.Request.URL.Path, r)
				dto.Error(c, http.StatusInternalServerError, "internal error")
			}
		}()
		c.Next()
	}
}

// Cors 跨域(allowOrigins 为空表示放行所有来源)。
func Cors(allowOrigins []string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(allowOrigins))
	for _, o := range allowOrigins {
		allowed[o] = struct{}{}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			if len(allowed) == 0 {
				c.Header("Access-Control-Allow-Origin", origin)
			} else if _, ok := allowed[origin]; ok {
				c.Header("Access-Control-Allow-Origin", origin)
			}
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
			c.Header("Access-Control-Allow-Headers", "Authorization,Content-Type,If-None-Match")
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// NoStore 私有/管理接口禁缓存。
func NoStore() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store")
		c.Next()
	}
}

// AuthToken 解析 Bearer token + 校验有效集合; 通过则注入 userID/role/token。
func AuthToken(us *service.UserService) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			dto.Error(c, http.StatusUnauthorized, "missing bearer token")
			return
		}
		token := strings.TrimSpace(auth[len("Bearer "):])
		if token == "" {
			dto.Error(c, http.StatusUnauthorized, "missing bearer token")
			return
		}
		claims, err := us.Authenticate(c.Request.Context(), token)
		if err != nil {
			dto.Error(c, http.StatusUnauthorized, "invalid or expired token")
			return
		}
		c.Set(CtxUserID, claims.UserID)
		c.Set(CtxRole, claims.Role)
		c.Set(CtxToken, token)
		// 刷新会话"最近活跃"(内部 60s 节流, 不增加每请求写放大)
		cache.TouchSession(c.Request.Context(), token)
		c.Next()
	}
}

// RequireAdmin 要求管理员角色。
func RequireAdmin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.GetInt(CtxRole) != 1 {
			dto.Error(c, http.StatusForbidden, "admin only")
			return
		}
		c.Next()
	}
}

// MaintToken 运维令牌（维护窗口接口专用）：只校验 X-Maint-Token 头，常量时间比较。
//
// 为什么要独立令牌而不是复用管理员 JWT：调它的是**部署脚本/agent**（服务器本地、无用户会话），
// 让脚本持有长期管理员 JWT 风险更大；这里只开放"暂停/恢复/查状态"三个运维动作。
// wanted 为空 ⇒ 一律拒绝（未配置即关闭该入口，绝不"没配就当放行"）。
func MaintToken(wanted string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !MaintTokenOK(c.GetHeader("X-Maint-Token"), wanted) {
			dto.Error(c, http.StatusForbidden, "维护令牌不匹配（X-Maint-Token）")
			return
		}
		c.Next()
	}
}

// MaintTokenOK 常量时间比较；未配置（wanted 为空）或未传（got 为空）恒 false。
// 单独抽出来是为了能单测"空配置=拒绝"这条 fail-closed 语义。
func MaintTokenOK(got, wanted string) bool {
	if wanted == "" || got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(wanted)) == 1
}

// RateLimit 令牌桶限流(per-IP, 走 Redis 协调态, 多副本一致)。
func RateLimit(scope string, capacity int, refillPerSec float64) gin.HandlerFunc {
	return func(c *gin.Context) {
		ok, err := cache.Allow(c.Request.Context(), scope, c.ClientIP(), capacity, refillPerSec)
		if err == nil && !ok {
			c.Header("Retry-After", "1")
			dto.Error(c, http.StatusTooManyRequests, "rate limited")
			return
		}
		c.Next()
	}
}
