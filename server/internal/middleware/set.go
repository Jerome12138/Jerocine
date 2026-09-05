// Package middleware 提供 gin 中间件: trace / recovery / cors / 鉴权 / 限流 / 缓存头。
package middleware

import (
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"server/internal/cache"
	"server/internal/dto"
	"server/internal/service"
)

// ctx keys
const (
	CtxUserID = "userID" // uint
	CtxRole   = "role"   // int
	CtxToken  = "token"  // string
)

// Trace 为每个请求生成 trace id(写 ctx + X-Trace-Id 头)。
func Trace() gin.HandlerFunc {
	return func(c *gin.Context) {
		dto.SetTraceId(c)
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
