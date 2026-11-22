package danmu

import (
	"fmt"
	"math/rand"
	"testing"
)

// 池索引与线性版**逐例等价**（2026-11-16 性能优化；2026-11-22 D10 改为「双锚互鉴」两映射）：
// 随机池 × 随机查询，索引版（`poolIndex.bridgeNum` / `dualAnchorDate`）必须与线性版
// （`BridgeNum` / `DualAnchorDate`）逐例相同。这是"提速 73×"能否安全上线的前提。

// randPool 造一个"像线上"的池：日期 + 期号 + 上/中/下 + 栏目 + 变体 随机组合
// （Num>0 ∧ Date≠"" 的组合即「双锚行」——同时进正查/反查两张映射）。
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
			// 正查（= 桥接）：索引版 vs 线性版
			if got, want := ix.bridgeNum(cur), BridgeNum(cur, pool); got != want {
				t.Fatalf("bridgeNum 不等价：cur=%+v got=%d want=%d（pool %d 条）", cur, got, want, len(pool))
			}
			// 反查（D10a 双锚互鉴）：索引版 vs 线性版
			if got, want := ix.dualAnchorDate(cur), DualAnchorDate(cur, pool); got != want {
				t.Fatalf("dualAnchorDate 不等价：cur=%+v got=%q want=%q", cur, got, want)
			}
			checked += 2
		}
	}
	if checked < 10000 {
		t.Fatalf("对照次数太少（%d），用例会空跑", checked)
	}
	// 反查的有效覆盖面：随机 borrower 命中映射的概率偏低 ⇒ 另用**池内双锚行自身**派生
	// n-only borrower（剥掉日期）做定向对照 —— 既保证命中，又天然覆盖"同 (s,v,n) 多日期冲突"形态。
	r2 := rand.New(rand.NewSource(20261122))
	revHit := 0
	for round := 0; round < 300; round++ {
		pool := randPool(r2, 1+r.Intn(60))
		ix := newPoolIndex(pool)
		for _, k := range pool {
			if k.Num <= 0 || k.Date == "" {
				continue
			}
			cur := EpisodeKey{Num: k.Num, Part: k.Part, Series: k.Series, Variant: k.Variant}
			got, want := ix.dualAnchorDate(cur), DualAnchorDate(cur, pool)
			if got != want {
				t.Fatalf("dualAnchorDate 不等价（定向）：cur=%+v got=%q want=%q", cur, got, want)
			}
			if got != "" {
				revHit++
			}
		}
	}
	if revHit < 50 {
		t.Fatalf("反查成功命中 %d 次，等价性对照面太小（随机池造不出双锚行？）", revHit)
	}
	t.Logf("已完成 %d 次逐例对照（其中反查成功命中 %d 次）", checked, revHit)
}

// 钉住两个已知语义点（防"顺手改语义"）：
//   - 正查（桥接）：候选期号**不唯一 ⇒ 0**；
//   - 反查（D10a）：候选日期**不唯一 ⇒ ""**；part 不容 ⇒ ""；borrower 带 d ⇒ 不适用。
func TestPoolIndex_Semantics(t *testing.T) {
	cur := EpisodeKey{Date: "20260810", Series: "王牌对王牌"}
	// 同日两个不同期号 ⇒ 正查放弃（锚点已退役 ⇒ 无兜底，落 D6-none）
	pool := []EpisodeKey{
		{Date: "20260810", Num: 7, Series: "王牌对王牌"},
		{Date: "20260810", Num: 8, Series: "王牌对王牌"},
	}
	if got := newPoolIndex(pool).bridgeNum(cur); got != 0 {
		t.Errorf("候选不唯一应放弃，得 %d", got)
	}
	// 反查：同 (s,v,n) 的双锚行给出两个不同日期 ⇒ 放弃（宁缺勿错）
	cur5 := EpisodeKey{Num: 5, Series: "王牌对王牌"} // n-only 行
	poolRev := []EpisodeKey{
		{Date: "20260803", Num: 5, Series: "王牌对王牌"},
		{Date: "20260810", Num: 5, Series: "王牌对王牌"},
	}
	if got := newPoolIndex(poolRev).dualAnchorDate(cur5); got != "" {
		t.Errorf("候选日期不唯一应放弃，得 %q", got)
	}
	// 唯一候选 ⇒ 借 d（且与线性版一致）
	poolRev1 := poolRev[:1]
	if got, want := newPoolIndex(poolRev1).dualAnchorDate(cur5), DualAnchorDate(cur5, poolRev1); got != want || got != "20260803" {
		t.Errorf("唯一候选应借 d=20260803（且与线性版一致），得 %q (want %q)", got, want)
	}
	// part 严格：borrower p=下、映射行 p=上 ⇒ 不借（数字序亦严格）
	if got := newPoolIndex([]EpisodeKey{{Date: "20260803", Num: 5, Part: "上", Series: "王牌对王牌"}}).
		dualAnchorDate(EpisodeKey{Num: 5, Part: "下", Series: "王牌对王牌"}); got != "" {
		t.Errorf("part 不容应不借，得 %q", got)
	}
	// borrower 自带日期 ⇒ 反查不适用（d 是硬证据，不覆盖）
	if got := newPoolIndex(poolRev).dualAnchorDate(EpisodeKey{Num: 5, Date: "20260901", Series: "王牌对王牌"}); got != "" {
		t.Errorf("自带日期不适用反查，得 %q", got)
	}
	// 类隔离：Series 不同 ⇒ 不借
	if got := newPoolIndex(poolRev1).dualAnchorDate(EpisodeKey{Num: 5, Series: "心动的周末"}); got != "" {
		t.Errorf("Series 不同应不借，得 %q", got)
	}
}
