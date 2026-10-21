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

// TestSaveUserTokenEvictsSession 设备数超限顶掉最旧设备时, 被顶 token 的会话元数据必须连带清掉 ——
// 否则「在线会话」一直显示这些已被顶掉的设备(直到 TTL 到期), 点"踢下线"也只是空操作。
func TestSaveUserTokenEvictsSession(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	SaveUserToken(ctx, 7, "tok-old", 2, time.Hour)
	SaveSession(ctx, "tok-old", 7, "alice", "", "", time.Hour)
	SaveUserToken(ctx, 7, "tok-mid", 2, time.Hour)
	SaveSession(ctx, "tok-mid", 7, "alice", "", "", time.Hour)

	// 第 3 台设备触发上限 → tok-old 被顶
	SaveUserToken(ctx, 7, "tok-new", 2, time.Hour)
	SaveSession(ctx, "tok-new", 7, "alice", "", "", time.Hour)

	// 三次登录在同一秒内(score 相同), 被顶的是 tok-old / tok-mid 中的哪一个由 ZSET 序决定;
	// 断言"恰好剩 2 条且含 tok-new"即可证明被顶者的会话被连带清理(否则会是 3 条)。
	list := ListSessions(ctx, 10)
	if len(list) != 2 {
		t.Fatalf("被顶设备的会话应消失, 剩 %d 条: %+v", len(list), list)
	}
	alive := map[string]bool{}
	for _, r := range list {
		alive[r.Token] = true
	}
	if !alive["tok-new"] {
		t.Fatalf("最新设备应在列: %+v", list)
	}
	if alive["tok-old"] && alive["tok-mid"] {
		t.Fatalf("被顶设备的会话没有被清理: %+v", list)
	}
}

// TestSaveSessionPrunesAgedIndex 登录时按年龄裁剪索引: 超过最长 TTL(400 天)的成员一定已过期,
// 应被顺手清掉, 防索引只增不减(列表侧只清"取到的前 N 条"的陈旧项, 够不到的老尾巴靠这里)。
func TestSaveSessionPrunesAgedIndex(t *testing.T) {
	mr := newTestCache(t)
	ctx := context.Background()
	ancient := float64(time.Now().Add(-500 * 24 * time.Hour).Unix())
	if _, err := mr.ZAdd(KeySessionIdx, ancient, "ghost"); err != nil {
		t.Fatalf("seed: %v", err)
	}
	SaveSession(ctx, "tok-now", 1, "u", "", "", time.Hour)
	members, _ := mr.ZMembers(KeySessionIdx)
	for _, m := range members {
		if m == "ghost" {
			t.Fatalf("超龄成员应被裁剪: %v", members)
		}
	}
}
