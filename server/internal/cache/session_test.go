package cache

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSessionLifecycle(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	const tok = "tok-abc-123"

	SaveSession(ctx, tok, 7, "alice", "1.2.3.4", "Mozilla/5.0", time.Hour)

	id := SessionID(tok)
	if id == "" || strings.Contains(id, tok) {
		t.Fatalf("会话 id 应是不可逆摘要且不含原文: %q", id)
	}
	list := ListSessions(ctx, 10)
	if len(list) != 1 {
		t.Fatalf("应有 1 条会话, got %d", len(list))
	}
	s := list[0]
	if s.ID != id || s.UserID != 7 || s.UserName != "alice" || s.IP != "1.2.3.4" || s.UA != "Mozilla/5.0" {
		t.Fatalf("会话字段不对: %+v", s)
	}
	if s.LoginAt == 0 || s.LastSeen != s.LoginAt {
		t.Fatalf("登录时间/最近活跃不对: %+v", s)
	}
	// 撤销用: 元数据里能取回 token 原文(SessionByID), 列表视图不带
	if got, ok := SessionByID(ctx, id); !ok || got.Token != tok {
		t.Fatalf("SessionByID 应能取回内部 token: ok=%v", ok)
	}

	DeleteSession(ctx, tok)
	if got := ListSessions(ctx, 10); len(got) != 0 {
		t.Fatalf("删除后应为空: %+v", got)
	}
}

// TestTouchSessionDoesNotResurrect 踢掉后即使再来一次 touch(节流表已清), 也不能把会话复活。
func TestTouchSessionDoesNotResurrect(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	const tok = "tok-gone"

	SaveSession(ctx, tok, 7, "alice", "", "", time.Hour)
	DeleteSession(ctx, tok)
	TouchSession(ctx, tok) // ZADD XX: 索引里没有 → 不创建
	if got := ListSessions(ctx, 10); len(got) != 0 {
		t.Fatalf("touch 不应复活已删除会话: %+v", got)
	}
}

// TestListSessionsPrunesStaleIndex 元数据过期(TTL 到)后, 索引里的残留成员应在列表时清掉。
func TestListSessionsPrunesStaleIndex(t *testing.T) {
	mr := newTestCache(t)
	ctx := context.Background()
	SaveSession(ctx, "tok-temp", 9, "bob", "", "", time.Hour)
	if got := ListSessions(ctx, 10); len(got) != 1 {
		t.Fatalf("准备数据失败: %+v", got)
	}
	// 只让元数据过期(删 hash), 保留索引 → 模拟 TTL 到期而索引尚未清理
	mr.Del(KeySession(SessionID("tok-temp")))
	if got := ListSessions(ctx, 10); len(got) != 0 {
		t.Fatalf("过期会话不应出现在列表: %+v", got)
	}
	if n := mr.Exists(KeySessionIdx); n {
		if members, _ := mr.ZMembers(KeySessionIdx); len(members) != 0 {
			t.Fatalf("索引残留成员应被清理: %v", members)
		}
	}
}

// TestClearOneTokenRemovesSession 登出/踢人两条路径都要连带删会话元数据。
func TestClearOneTokenRemovesSession(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	SaveUserToken(ctx, 7, "tok-a", 5, time.Hour)
	SaveSession(ctx, "tok-a", 7, "alice", "", "", time.Hour)

	ClearOneToken(ctx, 7, "tok-a")
	if got := ListSessions(ctx, 10); len(got) != 0 {
		t.Fatalf("ClearOneToken 应删会话: %+v", got)
	}
}
