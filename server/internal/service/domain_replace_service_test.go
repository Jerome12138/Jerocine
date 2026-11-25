package service

import (
	"testing"
	"time"

	"server/internal/macrep"
)

// 源站字段 → 我们库的落点，必须与真实库结构一致（movie.cover / episodes_json / 下载不存）。
func TestFieldTarget(t *testing.T) {
	pic := fieldTarget(macrep.FieldPic)
	if pic.Column != "movie.cover" || !pic.Executable {
		t.Fatalf("封面落点错误: %+v", pic)
	}
	play := fieldTarget(macrep.FieldPlay)
	if play.Column != "movie_play_source.episodes_json" || play.Executable {
		t.Fatalf("播放落点错误: %+v", play)
	}
	down := fieldTarget(macrep.FieldDown)
	if down.Column != "" || down.Executable {
		t.Fatalf("下载不应有落点/不可执行: %+v", down)
	}
	if unknown := fieldTarget("vod_area"); unknown.Column != "" || unknown.Executable || unknown.Note == "" {
		t.Fatalf("未知字段应无落点但要有说明: %+v", unknown)
	}
}

// 超长文案按**字符**截断，不能截出半个汉字。
func TestClipRunes(t *testing.T) {
	if got := clipRunes("域名替换", 2); got != "域名" {
		t.Fatalf("clipRunes = %q", got)
	}
	if got := clipRunes("abc", 10); got != "abc" {
		t.Fatalf("未超长不应改动: %q", got)
	}
}

// 公告日期只接受 2000-01-01 ~ 现在+1 天。
func TestValidAnnouncedAt(t *testing.T) {
	now := time.Now().UnixMilli()
	if !validAnnouncedAt(now) {
		t.Fatal("当前时间应合法")
	}
	if validAnnouncedAt(0) {
		t.Fatal("0 应判为非法")
	}
	old := time.Date(1999, 12, 31, 0, 0, 0, 0, time.Local).UnixMilli()
	if validAnnouncedAt(old) {
		t.Fatal("2000 年之前应判非法")
	}
	future := time.Now().Add(72 * time.Hour).UnixMilli()
	if validAnnouncedAt(future) {
		t.Fatal("超过 1 天的未来时间应判非法")
	}
}

// 来源 key / 完整 URL / 空值 三种入参都要能解析出可拉取地址。
func TestResolveDomainSource(t *testing.T) {
	if _, url := ResolveDomainSource(""); url == "" {
		t.Fatal("空入参应取默认来源")
	}
	src, url := ResolveDomainSource("lz")
	if src.Key != "lz" || url != DomainReplaceDefaultSource {
		t.Fatalf("按 key 解析错误: %+v %q", src, url)
	}
	custom, curl := ResolveDomainSource("https://example.com/a.html")
	if custom.Type != "custom" || curl != "https://example.com/a.html" {
		t.Fatalf("自定义地址解析错误: %+v %q", custom, curl)
	}
}

// 写操作后统计缓存必须失效（否则列表数字会停在旧值）。
func TestDomainStatsCacheInvalidate(t *testing.T) {
	domainStatsCache.mu.Lock()
	domainStatsCache.at = time.Now()
	domainStatsCache.counts = map[string]int64{"pending": 1}
	domainStatsCache.mu.Unlock()

	invalidateDomainStats()

	domainStatsCache.mu.Lock()
	defer domainStatsCache.mu.Unlock()
	if !domainStatsCache.at.IsZero() || domainStatsCache.counts != nil {
		t.Fatal("缓存未被清空")
	}
}
