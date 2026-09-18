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
