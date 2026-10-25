/**
 * danmu-service 壳单测（node:test 原生，无框架依赖；全程 mock，不触碰真实网络）。
 * 覆盖：三接口路由 / 秒→毫秒转换 / base64Data 适配 / 整集聚合与单段点播 / 降级。
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import { once } from 'node:events';
import { createApp } from '../server.js';
import { fetchComments, listEpisodes, listSegments, _setRegistryForTest } from '../lib/scraper.js';
import { createHttpAdapter } from '../lib/adapter.js';

function makeFakeScraper(overrides = {}) {
  return {
    getEpisodes: async () => [
      { provider: 'bilibili', episodeId: 'seasonId=44847&aid=1&cid=1091578122', episodeTitle: '正片', episodeNumber: 1 },
      { provider: 'bilibili', episodeId: 'seasonId=44847&aid=1&cid=1091578123', episodeTitle: '花絮', episodeNumber: 0 },
    ],
    getSegments: async () => [
      { provider: 'bilibili', segmentId: '1-1', startTime: 0 },
      { provider: 'bilibili', segmentId: '1-2', startTime: 360 },
    ],
    getComments: async (episodeId, segmentId) => [
      { id: 'a', content: '第一条', timestamp: 49.953, color: 0xffffff, mode: 1 },
      { id: 'b', content: '第二条', timestamp: 2, color: 0xff0000, mode: 4 },
    ],
    ...overrides,
  };
}

test('秒→毫秒转换：浮点秒 49.953 → 49953（B站），整数秒 2 → 2000（腾讯）', async () => {
  _setRegistryForTest({ scraperMap: { bilibili: makeFakeScraper() } });
  const comments = await fetchComments('bilibili', 'seasonId=44847&aid=1&cid=1091578122', '1-1');
  assert.equal(comments.length, 2);
  assert.equal(comments[0].time, 49953);
  assert.equal(comments[1].time, 2000);
  assert.equal(comments[0].text, '第一条');
  assert.equal(comments[0].mode, 1);
  assert.equal(comments[1].mode, 4);
});

test('整集聚合：不传 segmentId → 遍历所有段并合并（29 段场景），带 ≥200ms 节流', async () => {
  let segCalls = 0;
  let cmtCalls = 0;
  const t0 = Date.now();
  _setRegistryForTest({
    scraperMap: {
      bilibili: makeFakeScraper({
        getSegments: async () => Array.from({ length: 3 }, (_, i) => ({ segmentId: `s${i}`, startTime: i * 360 })),
        getComments: async () => {
          cmtCalls += 1;
          return [{ content: `弹幕${cmtCalls}`, timestamp: cmtCalls, color: 0xffffff, mode: 1 }];
        },
      }),
    },
  });
  const comments = await fetchComments('bilibili', 'seasonId=44847&aid=1&cid=1091578122');
  assert.equal(comments.length, 3);
  assert.equal(cmtCalls, 3);
  const elapsed = Date.now() - t0;
  assert.ok(elapsed >= 400, `应有两段间 200ms 节流，实测 ${elapsed}ms`);
  assert.equal(comments[2].time, 3000);
});

test('单段点播：传 segmentId → 只拉一段，不调 getSegments', async () => {
  let segCalls = 0;
  _setRegistryForTest({
    scraperMap: {
      tencent: makeFakeScraper({
        getSegments: async () => {
          segCalls += 1;
          return [];
        },
      }),
    },
  });
  const comments = await fetchComments('tencent', 'cid=mzc002003kpyd2m&vid=x4102k7l6gg', 't/v1/0/30000');
  assert.equal(comments.length, 2);
  assert.equal(segCalls, 0, '单段点播不应触发 getSegments');
});

test('listEpisodes：按集号匹配（episodeNumber=1 → 匹配正片）', async () => {
  _setRegistryForTest({ scraperMap: { bilibili: makeFakeScraper() } });
  const { episodes, matched } = await listEpisodes('bilibili', '44847', { episode: '1' });
  assert.equal(episodes.length, 2);
  assert.ok(matched, '应匹配到第 1 集');
  assert.equal(matched.episodeTitle, '正片');
});

test('listEpisodes：无对应集号 → matched 为 null（静默降级）', async () => {
  _setRegistryForTest({ scraperMap: { bilibili: makeFakeScraper() } });
  const { matched } = await listEpisodes('bilibili', '44847', { episode: '99' });
  assert.equal(matched, null);
});

test('listEpisodes：腾讯走 getEpisodesPage 分页（绕开上游 whole 过滤，正片 part=upper 可匹配）', async () => {
  const pageEps = [
    { provider: 'tencent', episodeId: 'cid=mzc&vid=d41024c1fwo', episodeTitle: '先导片上', episodeNumber: 0, episodePart: 'whole', episodeEdition: 'preview' },
    { provider: 'tencent', episodeId: 'cid=mzc&vid=s4102pomt63', episodeTitle: '第1期上：又争又抢', episodeNumber: 1, episodePart: 'upper', episodeEdition: 'main' },
    { provider: 'tencent', episodeId: 'cid=mzc&vid=r4102axkkku', episodeTitle: '第1期下：书房1v4', episodeNumber: 1, episodePart: 'lower', episodeEdition: 'main' },
    { provider: 'tencent', episodeId: 'cid=mzc&vid=x4102k7l6gg', episodeTitle: '第1期陪看上', episodeNumber: 1, episodePart: 'whole', episodeEdition: 'main' },
  ];
  _setRegistryForTest({
    scraperMap: {
      tencent: {
        getEpisodesPage: async () => pageEps,
      },
    },
  });
  const { episodes, matched } = await listEpisodes('tencent', 'mzc002003kpyd2m', { episode: '1', episodeName: '第1期上' });
  assert.equal(episodes.length, 4);
  assert.ok(matched, '应匹配到正片第1期上');
  assert.equal(matched.episodePart, 'upper');
  assert.equal(matched.episodeId, 'cid=mzc&vid=s4102pomt63');
});

test('listSegments 直通', async () => {
  _setRegistryForTest({ scraperMap: { bilibili: makeFakeScraper() } });
  const segs = await listSegments('bilibili', 'seasonId=44847&aid=1&cid=1091578122');
  assert.equal(segs.length, 2);
  assert.equal(segs[1].startTime, 360);
});

test('HTTP 路由：provider 不在白名单 → 400', async () => {
  const app = createApp();
  const srv = app.listen(0);
  await once(srv, 'listening');
  const port = srv.address().port;
  try {
    const res = await fetch(`http://127.0.0.1:${port}/danmu/iqiyi/episodes?resourceId=x`);
    assert.equal(res.status, 400);
    const body = await res.json();
    assert.match(body.error, /unsupported provider/);
  } finally {
    srv.close();
  }
});

test('HTTP 路由：未知 action → 404', async () => {
  const app = createApp();
  const srv = app.listen(0);
  await once(srv, 'listening');
  const port = srv.address().port;
  try {
    const res = await fetch(`http://127.0.0.1:${port}/danmu/bilibili/unknown`);
    assert.equal(res.status, 404);
  } finally {
    srv.close();
  }
});

test('HTTP 路由：episodes 接口走 fake scraper（秒→毫秒由 comments 处理）', async () => {
  _setRegistryForTest({ scraperMap: { bilibili: makeFakeScraper() } });
  const app = createApp();
  const srv = app.listen(0);
  await once(srv, 'listening');
  const port = srv.address().port;
  try {
    const res = await fetch(
      `http://127.0.0.1:${port}/danmu/bilibili/episodes?resourceId=44847&episode=1`,
    );
    assert.equal(res.status, 200);
    const body = await res.json();
    assert.equal(body.ok, true);
    assert.equal(body.data.matched.episodeTitle, '正片');
  } finally {
    srv.close();
  }
});

test('HTTP 路由：comments 接口整集聚合返回毫秒 time', async () => {
  _setRegistryForTest({ scraperMap: { bilibili: makeFakeScraper() } });
  const app = createApp();
  const srv = app.listen(0);
  await once(srv, 'listening');
  const port = srv.address().port;
  try {
    const res = await fetch(
      `http://127.0.0.1:${port}/danmu/bilibili/comments?episodeId=${encodeURIComponent('seasonId=44847&aid=1&cid=1091578122')}`,
    );
    assert.equal(res.status, 200);
    const body = await res.json();
    assert.equal(body.data.length, 4); // 2 段 × 2 条
    assert.equal(body.data[0].time, 49953);
  } finally {
    srv.close();
  }
});

test('降级：scraper 抛错 → HTTP 502（Go 侧再降级"暂无弹幕"）', async () => {
  _setRegistryForTest({
    scraperMap: {
      bilibili: makeFakeScraper({
        getComments: async () => {
          throw new Error('upstream boom');
        },
      }),
    },
  });
  const app = createApp();
  const srv = app.listen(0);
  await once(srv, 'listening');
  const port = srv.address().port;
  try {
    const res = await fetch(
      `http://127.0.0.1:${port}/danmu/bilibili/comments?episodeId=${encodeURIComponent('seasonId=44847&aid=1&cid=1091578122')}`,
    );
    assert.equal(res.status, 502);
  } finally {
    srv.close();
  }
});

test('adapter：base64Data=true → data 为 base64 字符串（B站 seg.so protobuf 通道）', async () => {
  const mockFetch = async () => {
    const buf = Buffer.from([0x0a, 0x02, 0x61, 0x62]); // 任意二进制
    return {
      status: 200,
      headers: new Map([['content-type', 'application/octet-stream']]),
      arrayBuffer: async () => buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength),
      text: async () => '',
    };
  };
  const adapter = createHttpAdapter({ fetchImpl: mockFetch });
  const res = await adapter.get('https://x/seg.so', { base64Data: true });
  assert.equal(res.statusCode, 200);
  assert.equal(res.data, Buffer.from([0x0a, 0x02, 0x61, 0x62]).toString('base64'));
});

test('adapter：JSON 响应正常解析（腾讯 JSON 路径）', async () => {
  const mockFetch = async () => ({
    status: 200,
    headers: new Map(),
    arrayBuffer: async () => new ArrayBuffer(0),
    text: async () => '{"a":1}',
  });
  const adapter = createHttpAdapter({ fetchImpl: mockFetch });
  const res = await adapter.get('https://x/api');
  assert.equal(res.data.a, 1);
});
