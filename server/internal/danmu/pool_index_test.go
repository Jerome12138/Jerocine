package danmu

import (
	"fmt"
	"math/rand"
	"testing"
)

// 池索引与线性版**逐例等价**（2026-11-16 性能优化）：随机池 × 随机查询，
// 索引版（`poolIndex.bridgeNum` / `anchorNum`）必须与线性版（`BridgeNum` / `AnchorNumForKey`）逐例相同。
// 这是"提速 73×"能否安全上线的前提。

// randPool 造一个"像线上"的池：日期 + 期号 + 上/中/下 + 栏目 + 变体 随机组合。
func randPool(r *rand.Rand, n int) []EpisodeKey {
	series := []string{"", "王牌对王牌", "心动的周末", "大人进度"}
	parts := []string{"", "上", "中", "下", "中上", "中下"}
	variants := []string{"", "先导", "特辑", "纯享"}
	out := make([]EpisodeKey, 0, n)
	for i := 0; i < n; i++ {
		k := EpisodeKey{}
		if r.Intn(3) > 0 {
			k.Date = fmt.Sprintf("2026%02d%02d", 1+r.Intn(12), 1+r.Intn(28))
		}
		if r.Intn(2) == 0 {
			k.Num = 1 + r.Intn(30)
		}
		k.Part = parts[r.Intn(len(parts))]
		k.Variant = variants[r.Intn(len(variants))]
		if r.Intn(3) == 0 {
			k.VariantNum = 1 + r.Intn(3)
		}
		k.Series = series[r.Intn(len(series))]
		out = append(out, k)
	}
	return out
}

func TestPoolIndex_EquivalentToLinear(t *testing.T) {
	r := rand.New(rand.NewSource(20261116))
	checked := 0
	for round := 0; round < 300; round++ {
		pool := randPool(r, 1+r.Intn(60))
		ix := newPoolIndex(pool)
		for q := 0; q < 20; q++ {
			cur := randPool(r, 1)[0]
			title := ""
			if cur.Date != "" {
				title = cur.Date // 裸日期行形态（触发 anchor 的 R9/R10 守卫）
			}
			if cur.Part != "" {
				title = cur.Date + cur.Part
			}
			// 桥接：索引版 vs 线性版
			if got, want := ix.bridgeNum(cur), BridgeNum(cur, pool); got != want {
				t.Fatalf("bridgeNum 不等价：cur=%+v got=%d want=%d（pool %d 条）", cur, got, want, len(pool))
			}
			// 锚点：索引版 vs 线性版
			if got, want := ix.anchorNum(title, cur), AnchorNumForKey(title, cur, pool); got != want {
				t.Fatalf("anchorNum 不等价：title=%q cur=%+v got=%d want=%d", title, cur, got, want)
			}
			checked += 2
		}
	}
	if checked < 10000 {
		t.Fatalf("对照次数太少（%d），用例会空跑", checked)
	}
	t.Logf("已完成 %d 次逐例对照", checked)
}

// 钉住两个已知语义点（防"顺手改语义"）：
//   - 桥接：候选期号**不唯一 ⇒ 0**；
//   - 锚点：同 Series 的「上」日期**<2 个 ⇒ 0**。
func TestPoolIndex_Semantics(t *testing.T) {
	cur := EpisodeKey{Date: "20260810", Series: "王牌对王牌"}
	// 同日两个不同期号 ⇒ bridge 放弃 ⇒ 落 anchor（此处池里没有「上」⇒ 0）
	pool := []EpisodeKey{
		{Date: "20260810", Num: 7, Series: "王牌对王牌"},
		{Date: "20260810", Num: 8, Series: "王牌对王牌"},
	}
	if got := newPoolIndex(pool).bridgeNum(cur); got != 0 {
		t.Errorf("候选不唯一应放弃，得 %d", got)
	}
	// 锚点：3 个「上」的日期，计数 = ≤ 本集日期的个数
	pool2 := []EpisodeKey{
		{Date: "20260803", Part: "上", Series: "王牌对王牌"},
		{Date: "20260810", Part: "上", Series: "王牌对王牌"},
		{Date: "20260817", Part: "上", Series: "王牌对王牌"},
	}
	p2 := []EpisodeKey{{Date: "20260824", Part: "下", Series: "王牌对王牌"}}
	if got, want := newPoolIndex(pool2).anchorNum("20260824下", p2[0]), AnchorNumForKey("20260824下", p2[0], pool2); got != want || got != 3 {
		t.Errorf("锚点计数应 3（且与线性版一致），得 %d (want %d)", got, want)
	}
	// 「上」只有 1 个 ⇒ 不猜
	pool3 := []EpisodeKey{{Date: "20260803", Part: "上", Series: "王牌对王牌"}}
	if got := newPoolIndex(pool3).anchorNum("20260824下", p2[0]); got != 0 {
		t.Errorf("「上」不足 2 个应不猜，得 %d", got)
	}
	// 裸日期行（无 p）不猜
	p4 := []EpisodeKey{{Date: "20260824"}}
	if got := newPoolIndex(pool2).anchorNum("20260824", p4[0]); got != 0 {
		t.Errorf("裸日期行应不猜，得 %d", got)
	}
}
