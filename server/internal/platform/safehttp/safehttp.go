// Package safehttp 提供防 SSRF 的出站 HTTP 客户端。
// 关键: 在 net.Dialer.Control 里对"实际要连的 IP"做拦截 —— 这发生在 DNS 解析之后、连接之前,
// 因此能同时挡住 ① 重定向跳到内网 ② DNS rebinding/TOCTOU(域名先解析成公网再改成内网)。
package safehttp

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"syscall"
	"time"
)

// ErrBlocked 目标解析到非公网地址。
var ErrBlocked = errors.New("safehttp: blocked non-public address")

// ErrBadScheme 非 http(s) 协议。
var ErrBadScheme = errors.New("safehttp: scheme must be http or https")

// NewClient 构造防 SSRF 客户端: dial 期拦截内网 IP + 重定向每跳校验协议。
func NewClient(timeout time.Duration) *http.Client {
	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return ErrBlocked
			}
			ip := net.ParseIP(host)
			if ip == nil || isBlockedIP(ip) {
				return ErrBlocked
			}
			return nil
		},
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:           dialer.DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			MaxIdleConns:          32,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("safehttp: too many redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return ErrBadScheme
			}
			return nil // 内网 IP 拦截由 dialer.Control 在每跳 dial 时兜底
		},
	}
}

// ValidateURL 预校验: 协议 http(s) + 主机存在 + 解析出的所有 IP 均为公网。
// 这是早期快速拒绝; dial 期 Control 才是防 rebinding 的最终防线。
func ValidateURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, ErrBadScheme
	}
	if u.Hostname() == "" {
		return nil, errors.New("safehttp: empty host")
	}
	if err := GuardHost(u.Hostname()); err != nil {
		return nil, err
	}
	return u, nil
}

// GuardHost 解析主机名并拒绝任何内网/环回/链路本地/未指定地址。
func GuardHost(host string) error {
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return ErrBlocked
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return ErrBlocked
		}
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified()
}
