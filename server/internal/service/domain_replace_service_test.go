package service

import (
	"strings"
	"testing"
)

// 钉住图床兜底列表的合并规则（2026-11-25 用户要求"替换逻辑里自动过滤掉相同的"）：
// 保序（第一项是主图床，语义不能变）+ 去重（忽略大小写）+ 忽略空串 + 上限 24。
func TestMergeImgHosts(t *testing.T) {
	if got := mergeImgHosts("p.lzp261118.com", []string{"p.lztutu.com", "P.LZP261118.COM"}); got != "p.lzp261118.com,p.lztutu.com" {
		t.Errorf("保序+去重失败: %q", got)
	}
	if got := mergeImgHosts("", []string{"img.a.com", "", "  ", "img.b.com"}); got != "img.a.com,img.b.com" {
		t.Errorf("忽略空串失败: %q", got)
	}
	if got := mergeImgHosts("  IMG.A.com  ,img.b.com", nil); got != "img.a.com,img.b.com" {
		t.Errorf("清理首尾空白/大小写失败: %q", got)
	}
	// 上限 24
	many := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		many = append(many, "h"+string(rune('a'+i%26))+".com")
	}
	got := mergeImgHosts("", many)
	if n := len(strings.Split(got, ",")); n > 24 {
		t.Errorf("未按上限截断: %d 个", n)
	}
}
