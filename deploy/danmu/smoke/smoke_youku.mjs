// 冒烟回归：优酷弹幕全链路（经壳逻辑 + 真实接口）
// 用例：流浪地球2 showId=efcdfcb66fd346f9b2a9（豆瓣 vendor youku）
// 回归点：① cookie jar（ensureTokenCookie 种 _m_h5_tk/_m_h5_tk_enc，弱 token 签名）
//         ② 多值 set-cookie 合并（undici entries() 只留最后一条 → adapter 合并逗号串，
//            否则丢 _m_h5_tk 导致 FAIL_SYS_TOKEN_EMPTY）
//         ③ segments 分段索引（约 174 段）
//         ④ comments 单段真实弹幕 + 毫秒契约
import assert from 'node:assert/strict';
import { listEpisodes, listSegments, fetchComments, ensureScraper } from '../lib/scraper.js';

const SHOW_ID = 'efcdfcb66fd346f9b2a9';

async function main() {
  await ensureScraper();

  // 1) episodes：匹配正片
  const { episodes, matched } = await listEpisodes('youku', SHOW_ID, { episode: 1 });
  assert.ok(episodes.length >= 1, `分集列表应 ≥1 条，实际 ${episodes.length}`);
  assert.ok(matched && matched.episodeId, '应匹配到正片且含 episodeId');
  console.log(`[smoke:youku] episodes=${episodes.length} matched="${matched.episodeTitle}" ✓`);

  // 2) segments：分段索引
  const segs = await listSegments('youku', matched.episodeId);
  assert.ok(segs.length >= 60, `正片分段应 ≥60 段，实际 ${segs.length}`);
  assert.ok(segs[0].startTime === 0 && segs[0].segmentId, '首段 startTime=0 且含 segmentId');
  console.log(`[smoke:youku] segments=${segs.length} ✓`);

  // 3) comments：单段真实弹幕，毫秒契约（弱 token 可过签名校验；个别段数据量少，取中段兜底）
  const probe = (await fetchComments('youku', matched.episodeId, segs[Math.floor(segs.length / 2)].segmentId));
  const comments = probe.length > 0 ? probe : (await fetchComments('youku', matched.episodeId, segs[0].segmentId));
  assert.ok(comments.length > 0, '应有真实弹幕');
  const c = comments[0];
  assert.equal(typeof c.text, 'string');
  assert.equal(typeof c.time, 'number');
  assert.ok(Number.isInteger(c.time), `time 应为整数毫秒，实际 ${c.time}`);
  assert.ok([1, 4, 5].includes(c.mode), `mode 应∈{1,4,5}，实际 ${c.mode}`);
  console.log(`[smoke:youku] comments=${comments.length} first="${c.text}" time=${c.time}ms ✓`);

  console.log('\n[smoke:youku] ALL PASS');
}

main().catch((e) => {
  console.error('[smoke:youku] FAIL:', e.message);
  process.exit(1);
});
