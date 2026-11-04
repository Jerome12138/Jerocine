package service

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/live"
)

func TestLiveSourceInputValidate(t *testing.T) {
	on := int8(entity.LiveSourceEnabled)
	off := int8(entity.LiveSourceDisabled)

	// 合法：默认启用
	e, err := LiveSourceInput{Name: "主源", URL: "https://example.com/list.m3u"}.validate()
	if err != nil {
		t.Fatalf("valid input: %v", err)
	}
	if e.State != entity.LiveSourceEnabled || e.Name != "主源" {
		t.Fatalf("default state/name: %+v", e)
	}
	// State 指针生效
	e, err = LiveSourceInput{Name: "a", URL: "http://x/y.m3u8", State: &off, Remark: " 备注 "}.validate()
	if err != nil || e.State != entity.LiveSourceDisabled || e.Remark != "备注" {
		t.Fatalf("state pointer/remark trim: %v %+v", err, e)
	}
	_ = on

	// 名称必填
	if _, err := (LiveSourceInput{Name: " ", URL: "http://x/y"}).validate(); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("missing name: %v", err)
	}
	// 地址仅 http(s)
	if _, err := (LiveSourceInput{Name: "a", URL: "rtmp://x/y"}).validate(); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("bad scheme: %v", err)
	}
	// 地址超长（列宽 1024）
	long := "http://x/" + strings.Repeat("a", 1100)
	if _, err := (LiveSourceInput{Name: "a", URL: long}).validate(); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("url too long: %v", err)
	}
	// 备注超长（列宽 255）
	if _, err := (LiveSourceInput{Name: "a", URL: "http://x", Remark: strings.Repeat("评", 256)}).validate(); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("remark too long: %v", err)
	}
	// 非法状态值
	bad := int8(7)
	if _, err := (LiveSourceInput{Name: "a", URL: "http://x", State: &bad}).validate(); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("bad state: %v", err)
	}
}

// fakeLiveStateRepo 内存频道覆盖层仓储（嵌入接口，只覆写被测方法）。
type fakeLiveStateRepo struct {
	repository.LiveChannelStateRepository
	states  []entity.LiveChannelState
	upserts []*entity.LiveChannelState
}

func (f *fakeLiveStateRepo) ListBySource(context.Context, int64) ([]entity.LiveChannelState, error) {
	return f.states, nil
}

func (f *fakeLiveStateRepo) Upsert(_ context.Context, st *entity.LiveChannelState) error {
	cp := *st
	f.upserts = append(f.upserts, &cp)
	return nil
}

// setupLiveCache 同 setupCache 范式指向 miniredis，但把实例交还调用方以便按键前缀断言
// 「缓存是否被动过」（setupCache 自身不返回 mr）。
func setupLiveCache(t *testing.T) *miniredis.Miniredis {
	t.Helper()
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(mr.Close)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	cache.Init(rdb, rdb)
	return mr
}

// TestRefreshSourceChannelsCacheSafety RefreshSourceChannels「先抓后失效」缓存安全
// （第3轮评审补测，对应第2轮 53080e8 的行为变更；8s 超时收紧后慢源刷新不得摧毁正缓存）：
//
//	a) 抓取成功 → Total=N 且该源频道缓存（解析+公开同前缀 v1:live:ch:<id>:*）被失效；
//	b) 抓取失败（上游 500）→ Total=0 且缓存键原样保留（不再触发"公开读回源失败→5min 负缓存空频道"）；
//	c) 抓到 0 频道空表（ParsePlaylist 对空表返回 nil，与抓取失败不可分）→ 同 b，不动缓存。
//
// 缓存可观察性：先经 LiveChannels 写入正缓存，再按 liveChanPatFmt 前缀列 miniredis 键比对。
func TestRefreshSourceChannelsCacheSafety(t *testing.T) {
	mr := setupLiveCache(t)
	ctx := context.Background()

	m3u := "#EXTM3U\n" +
		"#EXTINF:-1 group-title=\"央视\",CCTV-1\nhttp://s.example.com/cctv1.m3u8\n" +
		"#EXTINF:-1 group-title=\"卫视\",湖南卫视\nhttp://s.example.com/hntv.m3u8\n"
	goodSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(m3u))
	}))
	defer goodSrv.Close()
	deadSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer deadSrv.Close()
	emptySrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n")) // 无任何 EXTINF → 解析 0 频道
	}))
	defer emptySrv.Close()

	repo := &fakeLiveSourceRepo{byID: map[int64]*entity.LiveSource{
		1: {Id: 1, Name: "好源", URL: goodSrv.URL + "/list.m3u", State: entity.LiveSourceEnabled},
		2: {Id: 2, Name: "死源", URL: deadSrv.URL + "/dead.m3u", State: entity.LiveSourceEnabled},
		3: {Id: 3, Name: "空表源", URL: emptySrv.URL + "/empty.m3u", State: entity.LiveSourceEnabled},
	}}
	svc := NewLiveService(repo, &fakeLiveStateRepo{}, live.NewClient())

	// 三个源先经公开读写入正缓存
	for id := int64(1); id <= 3; id++ {
		if _, err := svc.LiveChannels(ctx, id); err != nil {
			t.Fatalf("预热源 %d 缓存: %v", id, err)
		}
	}
	keysOf := func(id int64) []string {
		prefix := fmt.Sprintf("v1:live:ch:%d:", id)
		var ks []string
		for _, k := range mr.Keys() { // miniredis v2 Keys() 无参, 全量取后按前缀过滤
			if strings.HasPrefix(k, prefix) {
				ks = append(ks, k)
			}
		}
		sort.Strings(ks)
		return ks
	}
	before := map[int64][]string{1: keysOf(1), 2: keysOf(2), 3: keysOf(3)}
	for id := int64(1); id <= 3; id++ {
		if len(before[id]) == 0 {
			t.Fatalf("源 %d 预热后应存在频道缓存键，实测无", id)
		}
	}

	// ---- a) 抓取成功 → 失效缓存 ----
	res, err := svc.RefreshSourceChannels(ctx, 1)
	if err != nil {
		t.Fatalf("成功源 refresh err = %v, want nil", err)
	}
	if res.Total != 2 {
		t.Fatalf("成功源 Total = %d, want 2", res.Total)
	}
	if len(keysOf(1)) != 0 {
		t.Fatalf("成功刷新后该源频道缓存应被失效，实测残留 %v", keysOf(1))
	}

	// ---- b) 抓取失败 → 不动缓存 ----
	resDead, err := svc.RefreshSourceChannels(ctx, 2)
	if err != nil {
		t.Fatalf("死源 refresh err = %v, want nil（失败静默返回 Total=0）", err)
	}
	if resDead.Total != 0 {
		t.Fatalf("死源 Total = %d, want 0", resDead.Total)
	}
	if fmt.Sprint(keysOf(2)) != fmt.Sprint(before[2]) {
		t.Fatalf("死源刷新失败不得动缓存: before=%v after=%v", before[2], keysOf(2))
	}

	// ---- c) 空表（0 频道）→ 与失败同路，不动缓存 ----
	resEmpty, err := svc.RefreshSourceChannels(ctx, 3)
	if err != nil {
		t.Fatalf("空表 refresh err = %v, want nil", err)
	}
	if resEmpty.Total != 0 {
		t.Fatalf("空表 Total = %d, want 0", resEmpty.Total)
	}
	if fmt.Sprint(keysOf(3)) != fmt.Sprint(before[3]) {
		t.Fatalf("空表刷新不得动缓存: before=%v after=%v", before[3], keysOf(3))
	}
}
