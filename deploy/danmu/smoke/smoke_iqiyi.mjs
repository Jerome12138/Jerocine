// 冒烟回归：爱奇艺弹幕全链路（经壳逻辑 + 真实接口）
// 用例：流浪地球2 entityId/tvid=7292991076670500（豆瓣 vendor iqiyi）
// 回归点：① zlib 流智能解压（cmts .z 无 Content-Encoding 头，Node fetch 不自动解压）
//         ② episodes 匹配正片（"正片｜流浪地球2"）
//         ③ segments 分段索引（每 5 分钟）
//         ④ comments 单段真实弹幕 + 毫秒契约
import assert from 'node:assert/strict';
import { listEpisodes, listSegments, fetchComments, ensureScraper } from '../lib/scraper.js';

const ENTITY_ID = '7292991076670500';

async function main() {
  await ensureScraper();

  // 1) episodes：匹配正片
  const { episodes, matched } = await listEpisodes('iqiyi', ENTITY_ID, { episode: 1 });
  assert.ok(episodes.length >= 1, `分集列表应 ≥1 条，实际 ${episodes.length}`);
  assert.ok(matched, '应匹配到正片');
  assert.ok(matched.episodeId, 'matched.episodeId 非空');
  console.log(`[smoke:iqiyi] episodes=${episodes.length} matched="${matched.episodeTitle}" ✓`);

  // 2) segments：分段索引
  const segs = await listSegments('iqiyi', matched.episodeId);
  assert.ok(segs.length >= 20, `正片分段应 ≥20 段，实际 ${segs.length}`);
  assert.ok(segs[0].startTime === 0 && segs[0].segmentId, '首段 startTime=0 且含 segmentId');
  console.log(`[smoke:iqiyi] segments=${segs.length} ✓`);

  // 3) comments：首段真实弹幕，毫秒契约（若首段量少取中段兜底）
  const probe = (await fetchComments('iqiyi', matched.episodeId, segs[0].segmentId));
  const comments = probe.length > 0 ? probe : (await fetchComments('iqiyi', matched.episodeId, segs[Math.floor(segs.length / 2)].segmentId));
  assert.ok(comments.length > 0, '应有真实弹幕');
  const c = comments[0];
  assert.equal(typeof c.text, 'string');
  assert.equal(typeof c.time, 'number');
  assert.ok(Number.isInteger(c.time), `time 应为整数毫秒，实际 ${c.time}`);
  assert.ok([1, 4, 5].includes(c.mode), `mode 应∈{1,4,5}，实际 ${c.mode}`);
  console.log(`[smoke:iqiyi] comments=${comments.length} first="${c.text}" time=${c.time}ms ✓`);

  console.log('\n[smoke:iqiyi] ALL PASS');
}

main().catch((e) => {
  console.error('[smoke:iqiyi] FAIL:', e.message);
  process.exit(1);
});
