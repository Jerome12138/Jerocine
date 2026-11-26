package douban

import (
	"testing"
	"time"
)

// 匹配原则：宁缺勿错 —— 名字一样但年份差太多（翻拍/重启）也不能选。
func TestPickSuggestion(t *testing.T) {
	list := []Suggestion{
		{Id: 1, Title: "伟大的长征", Year: 2026, Type: "tv"},
		{Id: 2, Title: "伟大征程", Year: 2021, Type: "tv"},
		{Id: 3, Title: "无名之辈", Year: 2018, Type: "movie"},
	}

	best, ok := PickSuggestion(list, "伟大的长征", 2026)
	if !ok || best.Id != 1 {
		t.Fatalf("精确同名同年应选中 1, got %+v ok=%v", best, ok)
	}

	// 年份差 1（跨年播出）仍可接受
	best, ok = PickSuggestion(list, "伟大的长征", 2025)
	if !ok || best.Id != 1 {
		t.Fatalf("年份差 1 应接受, got %+v ok=%v", best, ok)
	}

	// 名字只是包含、且年份对不上 → 拒绝
	if _, ok := PickSuggestion(list, "长征", 1999); ok {
		t.Fatal("名字包含但年份不符不应选中")
	}

	// 完全找不到
	if _, ok := PickSuggestion(list, "根本不存在的片", 2026); ok {
		t.Fatal("不存在的片不应匹配")
	}

	// 空候选
	if _, ok := PickSuggestion(nil, "伟大的长征", 2026); ok {
		t.Fatal("空候选不应匹配")
	}

	// 非法的 id 跳过
	if _, ok := PickSuggestion([]Suggestion{{Id: 0, Title: "伟大的长征", Year: 2026}}, "伟大的长征", 2026); ok {
		t.Fatal("id<=0 应跳过")
	}
}

// 同名但年份差 ≥2（重启/翻拍）必须放弃，交给人工。
func TestPickSuggestion_RemakeRejected(t *testing.T) {
	list := []Suggestion{
		{Id: 10, Title: "射雕英雄传", Year: 1983, Type: "tv"},
		{Id: 11, Title: "射雕英雄传", Year: 2017, Type: "tv"},
	}
	best, ok := PickSuggestion(list, "射雕英雄传", 2017)
	if !ok || best.Id != 11 {
		t.Fatalf("应选中同年那部, got %+v ok=%v", best, ok)
	}
	if _, ok := PickSuggestion(list, "射雕英雄传", 1995); ok {
		t.Fatal("两年份都差 ≥2 时应放弃")
	}
}

// 抖动区间：每次请求前应在 [3s,5s] 内随机，而不是固定值。
func TestNextGapJitter(t *testing.T) {
	c := &Client{}
	c.SetJitter(3*time.Second, 5*time.Second)
	seen := map[time.Duration]bool{}
	for i := 0; i < 200; i++ {
		g := c.nextGap()
		if g < 3*time.Second || g > 5*time.Second {
			t.Fatalf("间隔超出 [3s,5s]: %s", g)
		}
		seen[g] = true
	}
	if len(seen) < 5 {
		t.Fatalf("疑似固定值，200 次只出现 %d 种间隔", len(seen))
	}

	// SetInterval 应关闭抖动、回到固定值
	c.SetInterval(4 * time.Second)
	for i := 0; i < 20; i++ {
		if g := c.nextGap(); g != 4*time.Second {
			t.Fatalf("SetInterval 后应固定 4s, got %s", g)
		}
	}
}
