import type { FilmClass } from '@/types/manage'

/**
 * 分类级联勾选 → 服务端 catIds 生效集（祖先归一, 规格 D⑩）。
 *
 * 服务端契约(server/internal/domain/repository/query.go CatIDs +
 * repository/mysql/search_repo.go (pid IN ? OR cid IN ?)): 只做"影片自身 pid/cid 两列
 * ∈ 集合"的精确成员匹配, 不做祖先展开。影片归属两态(spider/engine.go backfillPid):
 *   - 子分类影片: pid=顶级祖先 id, cid=子分类 id
 *   - 直挂影片:   pid=0, cid=分类自身 id
 * 因此请求集合缺顶级 id 时, 直挂该顶级的影片(pid=0,cid=顶级)必漏。
 *
 * 归一三步(勾选 ∪ 全部子孙 ∪ 祖先补齐):
 * ① 勾选原样进生效集 —— 勾顶级传顶级 id(勾顶级命中其全部子分类影片 + 直挂影片, 既有契约);
 *    树上不存在的 id(树加载失败/陈旧勾选)也保留, 不丢用户筛选。
 * ② 勾中节点的全部子孙并入 —— 级联 UI(ManageCascader.toggle)本就整树勾选, 此步是纯函数
 *    自身语义完整的防御性展开, 不依赖调用方行为。
 * ③ 祖先补齐(自底向上, 传递闭包) —— 仅当祖先的全部子孙都已进生效集(被勾选覆盖)时补入:
 *    此时子分类影片本就全数命中, 补入只新增"直挂该祖先的影片", 无过匹配;
 *    存在未勾子孙(部分勾选)的祖先绝不反推补入 —— 否则 pid IN (父id) 会把未勾选
 *    兄弟子分类的影片一并命中(D⑩ 核查结论明确拒绝的反推方向)。
 */
export function normalizeCatIds(checked: ReadonlySet<number>, tree: readonly FilmClass[]): Set<number> {
  // ① 勾选原样保留(含树上找不到的 id)
  const effective = new Set(checked)
  // pre-order 摊平(父先于子), 反转即得自底向上序; tree 为空时循环不执行, 原样返回勾选
  const all: FilmClass[] = []
  const walk = (nodes: readonly FilmClass[]): void => {
    for (const n of nodes) {
      all.push(n)
      if (n.children?.length) walk(n.children)
    }
  }
  walk(tree)
  // ② 勾中节点的全部子孙并入
  for (const node of all) {
    if (effective.has(node.id)) {
      for (const id of strictDescendantIds(node, [])) effective.add(id)
    }
  }
  // ③ 祖先补齐: 自底向上遍历, 中层补齐后顶层可随之传递补齐
  for (const node of [...all].reverse()) {
    if (node.children?.length && !effective.has(node.id)) {
      const desc = strictDescendantIds(node, [])
      if (desc.every((id) => effective.has(id))) effective.add(node.id)
    }
  }
  return effective
}

/** 收集 node 的全部严格子孙 id(不含自身), 任意深度, 按 pre-order 追加进 out */
function strictDescendantIds(node: FilmClass, out: number[]): number[] {
  for (const child of node.children ?? []) {
    out.push(child.id)
    strictDescendantIds(child, out)
  }
  return out
}
