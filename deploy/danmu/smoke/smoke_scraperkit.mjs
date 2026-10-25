// 冒烟回归：腾讯弹幕全链路（经壳逻辑 + 真实接口）
// 用例：心动的信号第九季 cid=mzc002003kpyd2m，正片"第1期上"（POC 基线 vid=s4102pomt63）
// 回归点：① 腾讯分页列表必须含正片（上游 getEpisodes 的 whole 过滤/curated 分支会漏正片，壳已绕开）
//         ② 正片匹配（第1期上 → part=upper）
//         ③ 秒→毫秒转换 + 规范化契约
import assert from 'node:assert/strict';
import { listEpisodes, listSegments, fetchComments, ensureScraper } from '../lib/scraper.js';

const CID = 'mzc002003kpyd2m';
const EPISODE_NAME = '第1期上';

async function main() {
  await ensureScraper();

  // 1) episodes：完整列表含正片，matched 命中"第1期上"
  const { episodes, matched } = await listEpisodes('tencent', CID, { episode: 1, episodeName: EPISODE_NAME });
  assert.ok(episodes.length >= 30, `完整分集列表应 ≥30 条，实际 ${episodes.length}`);
  assert.ok(matched, '应匹配到正片（第1期上）');
  assert.equal(matched.episodeTitle, EPISODE_NAME + '：又争又抢，心动了就要打明牌！');
  assert.equal(matched.episodeId, `cid=${CID}&vid=s4102pomt63`);
  console.log(`[smoke:tencent] episodes=${episodes.length} matched=${matched.episodeTitle} vid=s4102pomt63 ✓`);

  // 2) segments：正片分段索引
  const segs = await listSegments('tencent', matched.episodeId);
  assert.ok(segs.length >= 60, `正片分段应 ≥60 段，实际 ${segs.length}`);
  assert.ok(segs[0].startTime === 0 && segs[0].segmentId, '首段 startTime=0 且含 segmentId');
  console.log(`[smoke:tencent] segments=${segs.length} ✓`);

  // 3) comments：首段真实弹幕，毫秒契约
  const comments = await fetchComments('tencent', matched.episodeId, segs[0].segmentId);
  assert.ok(comments.length > 0, '首段应有弹幕');
  const c = comments[0];
  assert.equal(typeof c.text, 'string');
  assert.equal(typeof c.time, 'number');
  assert.ok(Number.isInteger(c.time), `time 应为整数毫秒，实际 ${c.time}`);
  assert.ok(c.time >= 0 && c.time < 30000, `首段 time 应在 [0,30000)ms，实际 ${c.time}`);
  assert.ok([1, 4, 5].includes(c.mode), `mode 应∈{1,4,5}，实际 ${c.mode}`);
  console.log(`[smoke:tencent] comments(first seg)=${comments.length} first="${c.text}" time=${c.time}ms ✓`);

  console.log('\n[smoke:tencent] ALL PASS');
}

main().catch((e) => {
  console.error('[smoke:tencent] FAIL:', e.message);
  process.exit(1);
});
