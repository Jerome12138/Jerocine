// Package geoip 离线 IP 归属地查询(纯内存, 零外部依赖)。
//
// 数据文件: server/data/ip2region.db (约 8.7MB, 由 node-ip2region 2.3.0 分发解出)。
// 文件格式(与 node-ip2region 解析器一致):
//   - header(8B): firstIndexPtr uint32LE + lastIndexPtr uint32LE
//   - 索引区: 每条 12B = startIP(uint32LE) + endIP(uint32LE) + dataPtr(uint32LE)
//   - dataPtr 编码: 高 1 字节 = 数据长度, 低 24 位 = 数据偏移
//   - 数据块: 4B cityID(uint32LE) + 数据长度字节的 UTF-8 文本
//   - 文本: "国家|区域|省份|城市|ISP"
//
// 设计: 全局懒加载单例; 数据文件缺失/解析失败时 Search 返回空串, 不影响主流程。
package geoip

import (
	"encoding/binary"
	"net"
	"os"
	"strings"
	"sync"
)

const indexBlockLength = 12

// Searcher 内存态 IP 归属地查询器(并发安全: 只读)。
type Searcher struct {
	buf []byte
}

var (
	once     sync.Once
	searcher *Searcher
	initErr  error
)

// Init 加载数据文件(内部 once, 可重复调用)。
func Init(path string) error {
	once.Do(func() {
		searcher, initErr = newSearcher(path)
	})
	return initErr
}

func newSearcher(path string) (*Searcher, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(buf) < 16 {
		return nil, os.ErrInvalid
	}
	return &Searcher{buf: buf}, nil
}

// Search 返回 ip 的归属地(如 "广东省 深圳市"; 内网地址返回 "内网"; 查不到返回 "")。
func Search(ip string) string {
	if searcher == nil {
		return ""
	}
	return searcher.Search(ip)
}

// Search 查询单个 IP 的归属地。
func (s *Searcher) Search(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	ipNum := ip2uint(ip)
	if ipNum == 0 {
		return "" // 非法/非 IPv4
	}
	if isPrivate(ip) {
		return "内网"
	}
	region := s.memorySearch(ipNum)
	return formatRegion(region)
}

// memorySearch 二分索引(start/end 区间)定位 IP 并读取数据块文本。
func (s *Searcher) memorySearch(ipNum uint32) string {
	first := binary.LittleEndian.Uint32(s.buf[0:4])
	last := binary.LittleEndian.Uint32(s.buf[4:8])
	if last <= first {
		return ""
	}
	total := int((last-first)/indexBlockLength + 1)
	low, high := 0, total
	dataPos := uint32(0)
	for low <= high {
		mid := (low + high) >> 1
		pos := int(first) + mid*indexBlockLength
		if pos+indexBlockLength > len(s.buf) {
			break
		}
		sip := binary.LittleEndian.Uint32(s.buf[pos : pos+4])
		if ipNum < sip {
			high = mid - 1
			continue
		}
		eip := binary.LittleEndian.Uint32(s.buf[pos+4 : pos+8])
		if ipNum > eip {
			low = mid + 1
			continue
		}
		dataPos = binary.LittleEndian.Uint32(s.buf[pos+8 : pos+12])
		break
	}
	if dataPos == 0 {
		return ""
	}
	return s.readData(dataPos)
}

// readData 按 dataPtr 解码(高字节=长度, 低 24 位=偏移)读取文本(跳过 4B cityID)。
func (s *Searcher) readData(dataPtr uint32) string {
	dataLen := int((dataPtr >> 24) & 0xff)
	off := int(dataPtr & 0x00ffffff)
	if dataLen <= 0 || off+4+dataLen > len(s.buf) {
		return ""
	}
	return string(s.buf[off+4 : off+4+dataLen])
}

// formatRegion 把 "国家|区域|省份|城市|ISP" 格式化为展示文本。
func formatRegion(region string) string {
	if region == "" {
		return ""
	}
	parts := strings.Split(region, "|")
	if len(parts) < 5 {
		parts = append(parts, make([]string, 5-len(parts))...)
	}
	country, province, city := parts[0], parts[2], parts[3]
	clean := func(v string) string {
		if v == "0" || v == "" {
			return ""
		}
		return v
	}
	province, city = clean(province), clean(city)
	country = clean(country)
	switch {
	case country == "" || country == "中国" || country == "China":
		if province != "" && city != "" {
			return province + " " + city
		}
		if province != "" {
			return province
		}
		return city
	default:
		out := country
		if province != "" {
			out += " " + province
		}
		if city != "" {
			out += " " + city
		}
		return out
	}
}

// ip2uint 把 IPv4 字符串转成与 db 索引一致的数值(a<<24|b<<16|c<<8|d)。
func ip2uint(ip string) uint32 {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return 0
	}
	if v4 := parsed.To4(); v4 != nil {
		return uint32(v4[0])<<24 | uint32(v4[1])<<16 | uint32(v4[2])<<8 | uint32(v4[3])
	}
	return 0
}

// isPrivate 判断内网/保留地址(这些 IP 在 db 里没有有效归属地)。
func isPrivate(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return true
	}
	return parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() ||
		parsed.IsUnspecified() || parsed.IsMulticast() || parsed.IsLinkLocalMulticast()
}
