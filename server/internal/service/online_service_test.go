package service

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestOnlineService(t *testing.T) (*OnlineService, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewOnlineService(rdb), mr
}

func sess(sid, ip string, uid int64, watching bool) OnlineSession {
	return OnlineSession{Sid: sid, IP: ip, UID: uid, Watching: watching}
}

func TestOnlineService_UV_PV_Watching(t *testing.T) {
	s, _ := newTestOnlineService(t)
	ctx := context.Background()
	// 同 IP 三个会话(游客) → UV=1, PV=3
	s.Heartbeat(ctx, sess("s1", "1.1.1.1", 0, true))
	s.Heartbeat(ctx, sess("s2", "1.1.1.1", 0, false))
	s.Heartbeat(ctx, sess("s3", "1.1.1.1", 0, true))
	// 登录用户两个会话(uid=7) → UV+1
	s.Heartbeat(ctx, sess("s4", "2.2.2.2", 7, true))
	s.Heartbeat(ctx, sess("s5", "3.3.3.3", 7, false))
	// 另一游客另一 IP → UV+1
	s.Heartbeat(ctx, sess("s6", "4.4.4.4", 0, true))
	o := s.Overview(ctx)
	if o.PV != 6 {
		t.Fatalf("PV=%d want 6", o.PV)
	}
	if o.UV != 3 { // 游客IP1 + 用户7 + 游客IP4
		t.Fatalf("UV=%d want 3", o.UV)
	}
	if o.Watching != 4 { // s1,s3,s4(uid),s6
		t.Fatalf("Watching=%d want 4", o.Watching)
	}
	if len(o.Sessions) != 6 {
		t.Fatalf("明细行数=%d want 6", len(o.Sessions))
	}
}

func TestOnlineService_EmptySIDIgnored(t *testing.T) {
	s, _ := newTestOnlineService(t)
	s.Heartbeat(context.Background(), sess("", "1.1.1.1", 0, true))
	if o := s.Overview(context.Background()); o.PV != 0 || o.UV != 0 {
		t.Fatalf("空 sid 不应计数, got %+v", o)
	}
}

func TestOnlineService_ExpiryPurges(t *testing.T) {
	s, _ := newTestOnlineService(t)
	s.window = 20 * time.Millisecond
	s.Heartbeat(context.Background(), sess("s1", "1.1.1.1", 0, true))
	if o := s.Overview(context.Background()); o.PV != 1 {
		t.Fatalf("未过期应在线, got PV=%d", o.PV)
	}
	time.Sleep(30 * time.Millisecond)
	if o := s.Overview(context.Background()); o.PV != 0 {
		t.Fatalf("过期会话应被清理, got PV=%d", o.PV)
	}
}

func TestOnlineService_HeartbeatRefreshesFirstSeen(t *testing.T) {
	s, _ := newTestOnlineService(t)
	s.window = 40 * time.Millisecond
	ctx := context.Background()
	s.Heartbeat(ctx, sess("s1", "1.1.1.1", 0, true))
	time.Sleep(20 * time.Millisecond)
	s.Heartbeat(ctx, sess("s1", "1.1.1.1", 0, true)) // 续命
	time.Sleep(20 * time.Millisecond)
	o := s.Overview(ctx)
	if o.PV != 1 {
		t.Fatalf("续命后应仍在线, got PV=%d", o.PV)
	}
	if len(o.Sessions) != 1 || o.Sessions[0].FirstSeen > o.Sessions[0].LastSeen {
		t.Fatalf("firstSeen 应保持首次进入时间且不晚于 lastSeen, got %+v", o.Sessions[0])
	}
}

func TestOnlineService_SortedByLastSeenDesc(t *testing.T) {
	s, _ := newTestOnlineService(t)
	ctx := context.Background()
	s.Heartbeat(ctx, sess("s1", "1.1.1.1", 0, false))
	time.Sleep(10 * time.Millisecond)
	s.Heartbeat(ctx, sess("s2", "2.2.2.2", 0, false))
	o := s.Overview(ctx)
	if len(o.Sessions) != 2 || o.Sessions[0].Sid != "s2" || o.Sessions[1].Sid != "s1" {
		t.Fatalf("应按最近活跃倒序, got %+v", fmt.Sprintf("%+v", o.Sessions))
	}
}

func TestOnlineService_BotUA_Ignored(t *testing.T) {
	s, _ := newTestOnlineService(t)
	ctx := context.Background()
	// applebot: 用户实测在明细里见过的爬虫
	s.Heartbeat(ctx, OnlineSession{Sid: "b1", IP: "9.9.9.9", UA: "Mozilla/5.0 (compatible; Applebot/0.1; +http://www.apple.com/go/applebot)"})
	// googlebot / bytespider / curl
	s.Heartbeat(ctx, OnlineSession{Sid: "b2", IP: "9.9.9.10", UA: "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"})
	s.Heartbeat(ctx, OnlineSession{Sid: "b3", IP: "9.9.9.11", UA: "Mozilla/5.0 (compatible; Bytespider; spider-feedback@bytedance.com)"})
	s.Heartbeat(ctx, OnlineSession{Sid: "b4", IP: "9.9.9.12", UA: "curl/8.5.0"})
	// 正常浏览器不入 bot
	s.Heartbeat(ctx, OnlineSession{Sid: "u1", IP: "8.8.8.8", UA: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36"})
	o := s.Overview(ctx)
	if o.PV != 1 || o.UV != 1 || len(o.Sessions) != 1 {
		t.Fatalf("爬虫应全部忽略, 仅正常浏览器在线: got %+v", o)
	}
	if o.Today.UV != 1 || o.Today.PV != 1 {
		t.Fatalf("爬虫不应计入今日统计: got %+v", o.Today)
	}
}

func TestOnlineService_DailyStats(t *testing.T) {
	s, _ := newTestOnlineService(t)
	ctx := context.Background()
	// 两个不同游客 IP(UV=2, PV=2), 其中 1 人观看中
	s.Heartbeat(ctx, sess("s1", "1.1.1.1", 0, true))
	s.Heartbeat(ctx, sess("s2", "2.2.2.2", 0, false))
	o := s.Overview(ctx)
	if o.Today.UV != 2 || o.Today.PV != 2 {
		t.Fatalf("今日 UV/PV 应累计: got %+v", o.Today)
	}
	// 峰值: 当前在线 2 个会话, 应更新为 2
	if o.Today.Peak != 2 {
		t.Fatalf("今日峰值应为当前会话数 2: got %+v", o.Today)
	}
	// 同 sid 30min 内再次心跳不新增今日 PV; 新 sid 新增
	s.Heartbeat(ctx, sess("s1", "1.1.1.1", 0, true))
	s.Heartbeat(ctx, sess("s3", "3.3.3.3", 0, true))
	o = s.Overview(ctx)
	if o.Today.UV != 3 || o.Today.PV != 3 {
		t.Fatalf("s1 续心跳不应新增 PV, s3 应新增: got %+v", o.Today)
	}
	if o.Today.Peak != 3 {
		t.Fatalf("峰值应为 3: got %+v", o.Today)
	}
}

func TestOnlineService_SameSidDifferentIP_SeparateRows(t *testing.T) {
	s, _ := newTestOnlineService(t)
	ctx := context.Background()
	// 同一设备(sid=s1)先后从两个出口 IP 访问 → 应显示为两行会话
	s.Heartbeat(ctx, sess("s1", "1.1.1.1", 0, true))
	s.Heartbeat(ctx, sess("s1", "2.2.2.2", 0, false))
	o := s.Overview(ctx)
	if len(o.Sessions) != 2 {
		t.Fatalf("同 sid 不同 IP 应分两行, got %d 行", len(o.Sessions))
	}
	if o.PV != 2 {
		t.Fatalf("PV 应为 2 个会话行, got %d", o.PV)
	}
	// 游客按 IP 去重 → UV 仍为 2
	if o.UV != 2 {
		t.Fatalf("UV 应 2(两 IP), got %d", o.UV)
	}
	// 同一 sid+IP 续心跳仍合并为一行
	s.Heartbeat(ctx, sess("s1", "1.1.1.1", 0, true))
	o = s.Overview(ctx)
	if len(o.Sessions) != 2 || o.PV != 2 {
		t.Fatalf("同 sid+IP 续心跳应仍为 2 行, got %d 行 PV=%d", len(o.Sessions), o.PV)
	}
	// 登录用户同 sid 多 IP: 明细分行, UV 按 uid 去重为 1
	s.Heartbeat(ctx, sess("u1", "8.8.8.8", 9, true))
	s.Heartbeat(ctx, sess("u1", "9.9.9.9", 9, false))
	o = s.Overview(ctx)
	if len(o.Sessions) != 4 {
		t.Fatalf("登录用户多 IP 应分行, got %d 行", len(o.Sessions))
	}
	if o.UV != 3 { // 游客两 IP + 登录用户 u1
		t.Fatalf("UV 应 3, got %d", o.UV)
	}
}

func TestIsBotUAOnline(t *testing.T) {
	cases := []struct {
		ua   string
		want bool
	}{
		{"", false}, // 宽松版: 空 UA 不误伤
		{"Mozilla/5.0 (compatible; Applebot/0.1)", true},
		{"Mozilla/5.0 (compatible; Googlebot/2.1)", true},
		{"Mozilla/5.0 (compatible; Bytespider)", true},
		{"curl/8.5.0", true},
		{"Wget/1.21.4", true},
		{"python-requests/2.31.0", true},
		{"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 Chrome/126.0 Safari/537.36", false},
		{"Mozilla/5.0 (Linux; Android 10; TV) AppleWebKit/537.36 Chrome/126.0 Safari/537.36", false},
	}
	for _, c := range cases {
		if got := isBotUAOnline(c.ua); got != c.want {
			t.Fatalf("isBotUAOnline(%q)=%v want %v", c.ua, got, c.want)
		}
	}
}
