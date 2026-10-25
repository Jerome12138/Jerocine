// 冒烟回归：B站弹幕全链路（经壳逻辑 + 真实接口）
// 用例：流浪地球2 seasonId=44847（POC 基线 cid=1091578122，29 段，首段 499 条弹幕）
// 回归点：① B站 episodes 正常（上游无 whole 过滤问题）
//         ② 整集聚合路径（无 segmentId → 遍历 29 段串行，段间 ≥200ms）
//         ③ base64Data protobuf 通道（fetch adapter 自处理）
//         ④ 秒→毫秒转换
import assert from 'node:assert/strict';
import { listEpisodes, listSegments, fetchComments, ensureScraper } from '../lib/scraper.js';

const SEASON_ID = '44847';
const EPISODE_ID = `seasonId=${SEASON_ID}&aid=867288412&cid=1091578122`;

async function main() {
  await ensureScraper();

  // 1) episodes
  const { episodes, matched } = await listEpisodes('bilibili', SEASON_ID, { episode: 1 });
  assert.equal(episodes.length, 1, `B站该片应 1 条正片，实际 ${episodes.length}`);
  assert.equal(matched?.episodeTitle, '正片');
  console.log('[smoke:bilibili] episodes=1 matched=正片 ✓');

  // 2) segments：29 段
  const segs = await listSegments('bilibili', EPISODE_ID);
  assert.equal(segs.length, 29, `分段应为 29，实际 ${segs.length}`);
  console.log(`[smoke:bilibili] segments=${segs.length} ✓`);

  // 3) 单段点播（路径B）
  const single = await fetchComments('bilibili', EPISODE_ID, segs[0].segmentId);
  assert.ok(single.length > 0, '首段应有弹幕');
  assert.equal(typeof single[0].time, 'number');
  assert.ok(Number.isInteger(single[0].time), `time 应为毫秒整数，实际 ${single[0].time}`);
  console.log(`[smoke:bilibili] comments(first seg)=${single.length} first="${single[0].text}" time=${single[0].time}ms ✓`);

  // 4) 整集聚合（路径A，无 segmentId）：29 段串行聚合
  const started = Date.now();
  const agg = await fetchComments('bilibili', EPISODE_ID);
  const elapsed = Date.now() - started;
  assert.ok(agg.length >= 5000, `整集弹幕应 ≥5000 条，实际 ${agg.length}`);
  console.log(`[smoke:bilibili] comments(整集)=${agg.length} 耗时 ${elapsed}ms ✓`);

  console.log('\n[smoke:bilibili] ALL PASS');
}

main().catch((e) => {
  console.error('[smoke:bilibili] FAIL:', e.message);
  process.exit(1);
});
