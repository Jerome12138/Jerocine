/**
 * danmu-service 壳单测（node:test 原生，无框架依赖；全程 mock，不触碰真实网络）。
 * 覆盖：三接口路由 / 秒→毫秒转换 / base64Data 适配 / 整集聚合与单段点播 / 降级。
 */
import test from 'node:test';
import assert from 'node:assert/strict';
import { once } from 'node:events';
import { createApp } from '../server.js';
import {
  fetchComments,
  listEpisodes,
  listSegments,
  toIdString,
  ScraperError,
  _setRegistryForTest,
} from '../lib/scraper.js';
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
        // 单页语义：page 0 返回全量，page ≥1 超范围抛错（与线上腾讯一致；
        // 不再依赖 "length < 30" 猜页大小，见下方解耦用例）
        getEpisodesPage: async (cid, page) => {
          if (page >= 1) throw new Error('ret:0 超范围页');
          return pageEps;
        },
      },
    },
  });
  const { episodes, matched } = await listEpisodes('tencent', 'mzc002003kpyd2m', { episode: '1', episodeName: '第1期上' });
  assert.equal(episodes.length, 4);
  assert.ok(matched, '应匹配到正片第1期上');
  assert.equal(matched.episodePart, 'upper');
  assert.equal(matched.episodeId, 'cid=mzc&vid=s4102pomt63');
});

test('listEpisodes：腾讯分页不耦合上游 pageSize 常量（首页仅 2 条也不少拉）', async () => {
  // 模拟上游把 pageSize 从 30 改成 2：旧写法 "length < 30" 会在第一页就停，丢 4 集
  let calls = 0;
  _setRegistryForTest({
    scraperMap: {
      tencent: {
        getEpisodesPage: async (cid, page) => {
          calls += 1;
          if (page >= 3) throw new Error('ret:0 超范围页'); // 与线上一致：抛错收边
          return Array.from({ length: 2 }, (_, i) => ({
            provider: 'tencent',
            episodeId: `cid=mzc&vid=p${page}-${i}`,
            episodeTitle: `第${page * 2 + i + 1}集`,
            episodeNumber: page * 2 + i + 1,
            episodeEdition: 'main',
          }));
        },
      },
    },
  });
  const { episodes, matched } = await listEpisodes('tencent', 'mzc002003kpyd2m', { episode: '3' });
  assert.equal(episodes.length, 6, '3 页 × 2 条应全部拿到');
  assert.equal(calls, 4, '末页后多一次抛错探边（与线上同构）');
  assert.equal(matched.episodeNumber, 3);
});

test('listEpisodes：腾讯短页即末页（不再按 30 猜页大小）', async () => {
  let calls = 0;
  _setRegistryForTest({
    scraperMap: {
      tencent: {
        getEpisodesPage: async (cid, page) => {
          calls += 1;
          const sizes = [2, 2, 1];
          const size = sizes[page];
          if (size === undefined) throw new Error('ret:0 超范围页');
          return Array.from({ length: size }, (_, i) => ({
            provider: 'tencent',
            episodeId: `cid=mzc&vid=s${page}-${i}`,
            episodeTitle: `第${page * 2 + i + 1}期`,
            episodeNumber: page * 2 + i + 1,
            episodeEdition: 'main',
          }));
        },
      },
    },
  });
  const { episodes } = await listEpisodes('tencent', 'mzc002003kpyd2m', { episode: '5' });
  assert.equal(episodes.length, 5, '短页（1 条）即最后一页，应停在其后');
  assert.equal(calls, 3, '短页命中后不再多发请求');
});

test('listEpisodes：腾讯缺 getEpisodesPage → fail-fast 500（上游升级丢方法的静默故障防护）', async () => {
  _setRegistryForTest({
    scraperMap: {
      tencent: { getEpisodes: async () => [] }, // 模拟上游改 #private / 改名后的 dist
    },
  });
  await assert.rejects(
    () => listEpisodes('tencent', 'mzc002003kpyd2m', { episode: '1' }),
    (err) => {
      assert.ok(err instanceof ScraperError);
      assert.equal(err.status, 500);
      assert.match(err.message, /getEpisodesPage/);
      assert.match(err.message, /README/);
      return true;
    },
  );
});

test('signal：已中止 → listSegments/listEpisodes 抛 504（ABORT_MS 贯通三接口）', async () => {
  _setRegistryForTest({ scraperMap: { bilibili: makeFakeScraper(), tencent: { getEpisodesPage: async () => [] } } });
  const ac = new AbortController();
  ac.abort();
  await assert.rejects(() => listSegments('bilibili', 'seasonId=1', { signal: ac.signal }), (err) => {
    assert.equal(err.status, 504);
    return true;
  });
  await assert.rejects(() => listEpisodes('tencent', 'mzc002003kpyd2m', { signal: ac.signal }), (err) => {
    assert.equal(err.status, 504);
    return true;
  });
});

test('listSegments 直通', async () => {
  _setRegistryForTest({ scraperMap: { bilibili: makeFakeScraper() } });
  const segs = await listSegments('bilibili', 'seasonId=44847&aid=1&cid=1091578122');
  assert.equal(segs.length, 2);
  assert.equal(segs[1].startTime, 360);
});

test('toIdString：resourceId 按平台加前缀（腾讯 cid / B站 seasonId / 爱奇艺 entityId / 优酷 showId）', () => {
  assert.equal(toIdString('tencent', 'mzc002003kpyd2m'), 'cid=mzc002003kpyd2m');
  assert.equal(toIdString('bilibili', '44847'), 'seasonId=44847');
  assert.equal(toIdString('iqiyi', '229928700'), 'entityId=229928700');
  assert.equal(toIdString('youku', 'XNTk3MzA5NzgzMg=='), 'showId=XNTk3MzA5NzgzMg==');
});

test('HTTP 路由：provider 不在白名单 → 400', async () => {
  const app = createApp();
  const srv = app.listen(0);
  await once(srv, 'listening');
  const port = srv.address().port;
  try {
    // mgtv 上游支持但未启用（ID_PREFIX 无前缀）→ 400
    const res = await fetch(`http://127.0.0.1:${port}/danmu/mgtv/episodes?resourceId=x`);
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
  const body = Buffer.from('{"a":1}');
  const mockFetch = async () => ({
    status: 200,
    headers: new Map(),
    arrayBuffer: async () => body.buffer.slice(body.byteOffset, body.byteOffset + body.byteLength),
    text: async () => '{"a":1}',
  });
  const adapter = createHttpAdapter({ fetchImpl: mockFetch });
  const res = await adapter.get('https://x/api');
  assert.equal(res.data.a, 1);
});

test('adapter：zlib 流智能解压（爱奇艺 cmts .z 弹幕 XML 通道）', async () => {
  const zlib = await import('node:zlib');
  const xml = '<data><count>1</count><danmu><content>测试弹幕</content></danmu></data>';
  const compressed = zlib.deflateSync(Buffer.from(xml, 'utf8')); // zlib magic 0x78
  const mockFetch = async () => ({
    status: 200,
    headers: new Map([['content-type', 'application/octet-stream']]),
    arrayBuffer: async () => compressed.buffer.slice(compressed.byteOffset, compressed.byteOffset + compressed.byteLength),
    text: async () => '',
  });
  const adapter = createHttpAdapter({ fetchImpl: mockFetch });
  const res = await adapter.get('https://x/bullet.z');
  assert.equal(res.statusCode, 200);
  assert.equal(res.data, xml, 'zlib 流应被解压为 XML 文本（否则 fast-xml-parser 报嵌套超限）');
});

test('adapter：多值 Set-Cookie 合并为逗号分隔串（优酷 token 链：_m_h5_tk 与 _m_h5_tk_enc 都要收到）', async () => {
  const mockFetch = async () => {
    const h = new Headers();
    h.append('set-cookie', '_m_h5_tk=abc_123;Path=/;Domain=youku.com;Max-Age=86400');
    h.append('set-cookie', '_m_h5_tk_enc=def;Path=/;Domain=youku.com;Max-Age=86400');
    return {
      status: 200,
      headers: h,
      arrayBuffer: async () => new ArrayBuffer(0),
      text: async () => '{}',
    };
  };
  const adapter = createHttpAdapter({ fetchImpl: mockFetch });
  const res = await adapter.get('https://x/token');
  assert.ok(res.headers['set-cookie'].includes('_m_h5_tk=abc_123'), '第一条 set-cookie 不得丢失');
  assert.ok(res.headers['set-cookie'].includes('_m_h5_tk_enc=def'));
  assert.match(res.headers['set-cookie'], /,\s/, '应合并为逗号分隔（libs-fetch 按逗号 split 解析）');
});

test('adapter：Cookie 头原样透传、不凭空添加（跨站泄漏防回归）', async () => {
  // 上游 Fetch.buildHeaders 已把「实例 headers + 实例 cookie + 调用方 headers」合并进
  // options.headers，adapter 必须原样透传：显式 Cookie 不被篡改；无 Cookie 时不得
  // 凭空多一个头（单例 adapter 曾做 cookie jar，会把优酷 cna/_m_h5_tk 透传给腾讯域名）
  let seen;
  const mockFetch = async (_url, init) => {
    seen = init.headers;
    return { status: 200, headers: new Map(), arrayBuffer: async () => new ArrayBuffer(0) };
  };
  const adapter = createHttpAdapter({ fetchImpl: mockFetch });

  await adapter.get('https://x/a', { headers: { Cookie: 'cna=abc' } });
  assert.equal(seen.Cookie, 'cna=abc', '显式 Cookie 应原样透传');

  await adapter.get('https://x/b', { headers: { 'User-Agent': 'ua' } });
  assert.ok(!('Cookie' in seen), '上游未组装 Cookie 时 adapter 不得凭空添加');
  assert.equal(seen['User-Agent'], 'ua');
});
