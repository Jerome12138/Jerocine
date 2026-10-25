/**
 * danmu-service HTTP 壳（node:http，零框架依赖）。
 *
 * 路由：
 *   GET /danmu/:provider/episodes?resourceId=&episode=&episodeName=
 *   GET /danmu/:provider/segments?episodeId=          （episodeId 复合串，需 encodeURIComponent）
 *   GET /danmu/:provider/comments?episodeId=&segmentId=  （缺省 segmentId = 整集聚合）
 *
 * 约定：
 * - 单 provider 并发 1（withProviderLock）
 * - 请求级超时：ABORT_MS（默认 30s，覆盖整集聚合最坏 29 段×~350ms ≈ 10s）
 * - 不可用/异常 → JSON { error } + 对应状态码；上游错误静默转 502（Go 侧再降级"暂无弹幕"）
 */
import { createServer } from 'node:http';
import path from 'node:path';
import { fileURLToPath } from 'node:url';
import {
  assertProvider,
  ensureScraper,
  fetchComments,
  listEpisodes,
  listSegments,
  withProviderLock,
  ScraperError,
} from './lib/scraper.js';

const PORT = Number(process.env.PORT || 3611);
const HOST = process.env.HOST || '127.0.0.1';
const ABORT_MS = Number(process.env.ABORT_MS || 30000);

export function createApp() {
  return createServer(async (req, res) => {
  const url = new URL(req.url, `http://${req.headers.host}`);
  const m = url.pathname.match(/^\/danmu\/([a-z]+)\/(episodes|segments|comments)$/);
  if (!m) return json(res, 404, { error: 'not found' });
  const [, provider, action] = m;
  const q = url.searchParams;

  try {
    assertProvider(provider);
    await ensureScraper();
    const ac = new AbortController();
    const timer = setTimeout(() => ac.abort(), ABORT_MS);

    const payload = await withProviderLock(provider, async () => {
      switch (action) {
        case 'episodes':
          return await listEpisodes(provider, q.get('resourceId') ?? '', {
            episode: q.get('episode') ?? undefined,
            episodeName: q.get('episodeName') ?? undefined,
          });
        case 'segments':
          return await listSegments(provider, q.get('episodeId') ?? '');
        case 'comments':
          return await fetchComments(provider, q.get('episodeId') ?? '', q.get('segmentId') ?? undefined, {
            signal: ac.signal,
          });
        default:
          throw new ScraperError(404, 'unknown action');
      }
    });
    clearTimeout(timer);
    json(res, 200, { ok: true, data: payload });
  } catch (err) {
    const status = err instanceof ScraperError ? err.status : 502;
    if (status >= 500) console.error(`[danmu] ${provider}/${action} failed:`, err?.message);
    json(res, status, { ok: false, error: err?.message || 'internal error' });
  }
  });
}

// 仅直接运行时监听（node server.js）；被单测 import 时不占用端口
const isMain = process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url);
if (isMain) {
  const server = createApp();
  server.listen(PORT, HOST, () => {
    console.log(`[danmu] listening on http://${HOST}:${PORT} (abort ${ABORT_MS}ms)`);
  });
}

function json(res, status, body) {
  res.writeHead(status, { 'Content-Type': 'application/json; charset=utf-8' });
  res.end(JSON.stringify(body));
}
