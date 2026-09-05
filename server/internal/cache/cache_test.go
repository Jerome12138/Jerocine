package cache

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestCache(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	Init(rdb, rdb) // 测试中 cache/coord 共用一实例
	return mr
}

type box struct {
	V string `json:"v"`
}

func TestGetOrLoad_HitMissNegative(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	var calls int32

	load := func(found bool) func(context.Context) (box, bool, error) {
		return func(context.Context) (box, bool, error) {
			atomic.AddInt32(&calls, 1)
			if !found {
				return box{}, false, nil
			}
			return box{V: "hello"}, true, nil
		}
	}

	// miss → loader 跑一次, 命中后不再跑
	v, found, err := GetOrLoad(ctx, "k1", time.Minute, load(true))
	if err != nil || !found || v.V != "hello" {
		t.Fatalf("first load: v=%v found=%v err=%v", v, found, err)
	}
	v, found, _ = GetOrLoad(ctx, "k1", time.Minute, load(true))
	if !found || v.V != "hello" {
		t.Fatalf("second should hit cache: %v %v", v, found)
	}
	if calls != 1 {
		t.Fatalf("loader should run once, got %d", calls)
	}

	// 负缓存: 不存在 → found=false, 再查不再调 loader
	atomic.StoreInt32(&calls, 0)
	_, found, _ = GetOrLoad(ctx, "k2", time.Minute, load(false))
	if found {
		t.Fatal("expected not found")
	}
	_, found, _ = GetOrLoad(ctx, "k2", time.Minute, load(false))
	if found || calls != 1 {
		t.Fatalf("negative cache miss: found=%v calls=%d", found, calls)
	}
}

func TestGetOrLoad_Singleflight(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	var calls int32
	loader := func(context.Context) (box, bool, error) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(20 * time.Millisecond) // 放大并发窗口
		return box{V: "x"}, true, nil
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, found, err := GetOrLoad(ctx, "sf", time.Minute, loader)
			if err != nil || !found || v.V != "x" {
				t.Errorf("sf result: %v %v %v", v, found, err)
			}
		}()
	}
	wg.Wait()
	if c := atomic.LoadInt32(&calls); c > 3 {
		t.Fatalf("singleflight should collapse loads, got %d", c)
	}
}

func TestDelAndPattern(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	one := func(context.Context) (box, bool, error) { return box{V: "1"}, true, nil }
	GetOrLoad(ctx, KeyMovieDetail(1), time.Minute, one)
	GetOrLoad(ctx, KeyMovieDetail(2), time.Minute, one)
	DelByPattern(ctx, PatMovie)
	var calls int32
	GetOrLoad(ctx, KeyMovieDetail(1), time.Minute, func(context.Context) (box, bool, error) {
		atomic.AddInt32(&calls, 1)
		return box{V: "1"}, true, nil
	})
	if calls != 1 {
		t.Fatalf("pattern del should force reload, calls=%d", calls)
	}
}

func TestAllowTokenBucket(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	// 容量 2, 每秒补 1 → 头两次放行, 第三次拒绝
	a1, err := Allow(ctx, "m3u8", "1.1.1.1", 2, 1)
	if err != nil {
		t.Fatalf("Allow err (miniredis Lua?): %v", err)
	}
	a2, _ := Allow(ctx, "m3u8", "1.1.1.1", 2, 1)
	a3, _ := Allow(ctx, "m3u8", "1.1.1.1", 2, 1)
	if !a1 || !a2 || a3 {
		t.Fatalf("token bucket: %v %v %v (want true true false)", a1, a2, a3)
	}
	// 不同 ip 独立
	if ok, _ := Allow(ctx, "m3u8", "2.2.2.2", 2, 1); !ok {
		t.Fatal("different ip should have own bucket")
	}
}

func TestTryLockUnlock(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	tok, ok, err := TryLock(ctx, KeyLockCron("t1"), time.Minute)
	if err != nil || !ok || tok == "" {
		t.Fatalf("first lock: ok=%v tok=%q err=%v", ok, tok, err)
	}
	if _, ok2, _ := TryLock(ctx, KeyLockCron("t1"), time.Minute); ok2 {
		t.Fatal("second lock should fail while held")
	}
	Unlock(ctx, KeyLockCron("t1"), tok)
	if _, ok3, _ := TryLock(ctx, KeyLockCron("t1"), time.Minute); !ok3 {
		t.Fatal("lock should be acquirable after unlock")
	}
}

func TestJobProgress(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	if err := JobStart(ctx, "src_lz", "HD(lz)", 10); err != nil {
		t.Fatalf("JobStart: %v", err)
	}
	JobIncrDone(ctx, "src_lz", 3)
	JobIncrFailed(ctx, "src_lz", 1)
	p, _ := JobSnapshot(ctx, "src_lz")
	if p.Total != 10 || p.Done != 3 || p.Failed != 1 || p.State != JobRunning || p.Name != "HD(lz)" {
		t.Fatalf("snapshot mismatch: %+v", p)
	}
	JobSetState(ctx, "src_lz", JobPaused)
	if JobReadState(ctx, "src_lz") != JobPaused {
		t.Fatal("state should be paused")
	}
	list, _ := JobList(ctx)
	if len(list) != 1 || list[0].SourceId != "src_lz" {
		t.Fatalf("job list: %+v", list)
	}
}

func TestUserTokenResilience(t *testing.T) {
	mr := newTestCache(t)
	ctx := context.Background()
	uid, tok := int64(42), "tok-abc"
	if err := SaveUserToken(ctx, uid, tok, 5, time.Hour); err != nil {
		t.Fatalf("SaveUserToken: %v", err)
	}
	if !IsUserTokenValid(ctx, uid, tok) {
		t.Fatal("token should be valid after save")
	}
	// 集合不应带 TTL(否则会被 volatile-lru 淘汰致掉登录 —— 这是重部署掉登录的真正修复)
	if ttl := mr.TTL(KeyToken(uid)); ttl != 0 {
		t.Fatalf("token key should have no TTL, got %v", ttl)
	}
	// 登出(清当前 token → 集合空键消失) → fail-closed 判无效(撤销生效)
	ClearOneToken(ctx, uid, tok)
	if IsUserTokenValid(ctx, uid, tok) {
		t.Fatal("token must be rejected after logout (fail-closed)")
	}
	// 安全关键: 整库被清(wipe) → redis.Nil → 一律拒, 绝不放行/回填(否则会话撤销被永久绕过)
	if err := SaveUserToken(ctx, uid, tok, 5, time.Hour); err != nil {
		t.Fatalf("re-save: %v", err)
	}
	mr.FlushAll()
	if IsUserTokenValid(ctx, uid, tok) {
		t.Fatal("token MUST be rejected after redis wipe (no fail-open bypass)")
	}
	// Redis 连接不可达(临时网络错误) → 暂放行靠 JWT 兜底(不回填)
	mr.Close()
	if !IsUserTokenValid(ctx, uid, tok) {
		t.Fatal("token should fail-open on redis connection error")
	}
}

func TestDeviceAuthFlow(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	dc, uc := "devcode-xyz", "ACDE2345"
	if err := SaveDeviceCode(ctx, dc, uc, time.Minute); err != nil {
		t.Fatalf("SaveDeviceCode: %v", err)
	}
	da, err := GetDeviceCode(ctx, dc)
	if err != nil || da == nil || da.Status != DeviceCodePending || da.UserCode != uc {
		t.Fatalf("get pending: %+v err=%v", da, err)
	}
	ok, err := AuthorizeByUserCode(ctx, uc, 7, time.Minute)
	if err != nil || !ok {
		t.Fatalf("authorize: ok=%v err=%v", ok, err)
	}
	da, _ = GetDeviceCode(ctx, dc)
	if da == nil || da.Status != DeviceCodeAuthorized || da.UserID != 7 {
		t.Fatalf("after authorize: %+v", da)
	}
	if ok, _ := AuthorizeByUserCode(ctx, "NOPE9999", 7, time.Minute); ok {
		t.Fatal("authorize unknown userCode should be false")
	}
	DeleteDeviceCode(ctx, dc, uc)
	if _, err := GetDeviceCode(ctx, dc); err == nil {
		t.Fatal("device code should be gone after delete")
	}
}

func TestSpiderTemp(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	items := []ScoredMember{{Score: 1, Member: `{"mid":1}`}, {Score: 3, Member: `{"mid":3}`}, {Score: 2, Member: `{"mid":2}`}}
	if err := SpiderTempPush(ctx, "src_lz", items); err != nil {
		t.Fatalf("push: %v", err)
	}
	got, _ := SpiderTempPopMax(ctx, "src_lz", 2)
	if len(got) != 2 || got[0] != `{"mid":3}` {
		t.Fatalf("popmax should return highest scores first: %v", got)
	}
	SpiderTempClear(ctx, "src_lz")
	rest, _ := SpiderTempPopMax(ctx, "src_lz", 10)
	if len(rest) != 0 {
		t.Fatalf("clear should empty temp, got %v", rest)
	}
}
