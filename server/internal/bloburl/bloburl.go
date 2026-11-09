// Package bloburl 负责「本地 blob 资源」的存储形态与对外 URL 之间的转换。
//
// 设计(2026-11-09, 方案2):
//   - 数据库只存**裸 key**(如 backdrop/1.webp、poster/123.jpg、gallery/xxx.jpg), 不含任何站点前缀;
//   - 对外出参时按配置的基址拼成完整 URL: 基址可为根相对路径(/filestore/)或绝对域名(https://img.xxx/);
//   - 由此"换对外路径 / 换 CDN 独立域名"只需改一处配置(BLOB_BASE_URL), **零数据迁移**。
//
// 放行规则: 空串、含 "://" 的外链、以 "/" 开头的根相对值(旧数据/过渡期)、不含 "/" 的值(如 "-" 哨兵)
// 一律原样返回 —— 即使个别地方漏 strip 存了带前缀的旧值, 页面也不会裂(只是那一行不随基址迁移)。
package bloburl

import "strings"

// base 对外基址(BLOB_BASE_URL)。启动时注入一次, 运行期不变。
var base string

// SetBase 注入对外基址, 如 "/filestore/" 或 "https://img.example.com/"。
func SetBase(b string) { base = b }

// Base 返回当前对外基址(前端经 /config/site 的 fileBase 字段消费, 用于路径模式下的跨域解析)。
func Base() string { return base }

// legacyPrefixes 历史/过渡前缀: 写入时若遇到这些根相对前缀, 一并剥成裸 key。
var legacyPrefixes = []string{"/api/upload/", "/filestore/", "/statics/"}

// Resolve 存储值(裸 key 或外链) → 对外 URL。
func Resolve(v string) string {
	if v == "" || base == "" {
		return v
	}
	if strings.Contains(v, "://") { // 外链原样
		return v
	}
	if strings.HasPrefix(v, "/") { // 已是根相对(旧值/过渡), 交给 nginx 直服
		return v
	}
	if !strings.Contains(v, "/") { // 无斜杠(如 "-" 哨兵), 非 key
		return v
	}
	return strings.TrimRight(base, "/") + "/" + v
}

// ToKey 对外 URL → 存储值(裸 key)。外链、非 blob 形态原样返回。
func ToKey(v string) string {
	if v == "" {
		return v
	}
	// 先按配置基址剥(路径或绝对域都适用 —— 绝对域的值含 "://", 必须在该判定之前处理)。
	if base != "" {
		b := strings.TrimRight(base, "/") + "/"
		if strings.HasPrefix(v, b) {
			return strings.TrimPrefix(v, b)
		}
	}
	if strings.Contains(v, "://") { // 其它外链原样
		return v
	}
	if strings.HasPrefix(v, "/") { // 旧前缀的根相对值 → 剥成裸 key
		for _, p := range legacyPrefixes {
			if strings.HasPrefix(v, p) {
				return strings.TrimPrefix(v, p)
			}
		}
	}
	return v
}
