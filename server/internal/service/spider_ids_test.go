package service

import "testing"

// TestChunkIds 防呆（000052 定向补采通道）：分片必须
//   - **跳过非正数**（与引擎 `joinIds` 同口径：0/负数不该进 maccms 的 ids 参数）；
//   - 尾批取余且**不丢 id**（丢一个 ⇒ 那部片的 `source_name` 永远补不上，且没人报错 ✗）；
//   - `size<=0` 或大于总数 ⇒ 退化为单批；
//   - 空输入 / 全非正数 ⇒ `nil`（不发请求）。
//
// 为什么必须钉住：引擎 `CollectByIds` 把 ids 拼进 maccms 的 **query string**（`ac=detail&ids=…`）且
// **自身不分片** ⇒ 分片是"不撞 URL 长度 / 上游容忍度"的唯一保障，也是 5 万+ id 定向补采的可行性前提。
func TestChunkIds(t *testing.T) {
	got := chunkIds([]int64{1, 2, 3, 4, 5, 6, 7}, 3)
	if len(got) != 3 {
		t.Fatalf("7 个 id / 批 3 ⇒ 应 3 批，得 %d：%v", len(got), got)
	}
	if len(got[0]) != 3 || len(got[2]) != 1 {
		t.Errorf("尾批应取余（3/3/1）：%v", got)
	}
	total := 0
	for _, c := range got {
		total += len(c)
	}
	if total != 7 {
		t.Errorf("分片后总数应等于 7，得 %d（丢 id ⇒ 补采永远补不齐）", total)
	}

	if g := chunkIds([]int64{0, -1, 5, 6}, 10); len(g) != 1 || len(g[0]) != 2 || g[0][0] != 5 {
		t.Errorf("非正数应被跳过、size 大于总数应退化单批：%v", g)
	}
	if g := chunkIds([]int64{1, 2}, 0); len(g) != 1 || len(g[0]) != 2 {
		t.Errorf("size<=0 应退化单批：%v", g)
	}
	if g := chunkIds(nil, 100); g != nil {
		t.Errorf("空输入应返回 nil：%v", g)
	}
	if g := chunkIds([]int64{0, -3}, 100); g != nil {
		t.Errorf("全非正数应返回 nil：%v", g)
	}
	if n := countPositive([]int64{0, 1, -2, 3}); n != 2 {
		t.Errorf("countPositive=%d, want 2", n)
	}
}
