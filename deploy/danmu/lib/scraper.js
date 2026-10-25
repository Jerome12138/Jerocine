/**
 * scraper-kit 初始化与 provider 路由（弹幕数据层胶水）。
 *
 * 约定（集成方案 §8）：
 * - resourceId 收裸值，壳内按 provider 加前缀（tencent→`cid=`、bilibili→`seasonId=`）
 * - timestamp 上游为秒（B站浮点），壳是唯一秒→毫秒转换点（Math.round(t*1000)）
 * - 段间节流 ≥200ms、单 provider 并发 1
 * - 只启用 bilibili / tencent（上游还支持 iqiyi/youku/mgtv/renren，启用时加白名单一行）
 * - 腾讯分页依赖上游 getEpisodesPage（TS 层 private，dist 运行时可用；src 已打本地
 *   补丁显式公开，升级上游需重新应用 —— 见 listEpisodes 内的 fail-fast 守卫）
 */
import { pathToFileURL } from 'node:url';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const SCRAPER_KIT_URL = pathToFileURL(
  path.join(__dirname, '..', 'upstream', 'packages', 'scraper-kit', 'dist', 'index.js'),
).href;

const ID_PREFIX = {
  bilibili: 'seasonId=',
  tencent: 'cid=',
};
const ENABLED_PROVIDERS = Object.keys(ID_PREFIX);
const SEGMENT_GAP_MS = 200; // 段间节流（§8.8，与 Go 侧纪律一致）

let registry = null;

/** 单测注入钩子：替换 registry（不触碰真实网络） */
export function _setRegistryForTest(fakeRegistry) {
  registry = fakeRegistry;
}

export async function ensureScraper() {
  if (registry) return registry;
  const { createScraperRegistry, initializeFetchAdapter, initializeFetchStorageAdapter } = await import(SCRAPER_KIT_URL);
  const { createHttpAdapter, createMemoryStorageAdapter } = await import('./adapter.js');
  initializeFetchAdapter(createHttpAdapter());
  initializeFetchStorageAdapter(createMemoryStorageAdapter());
  registry = createScraperRegistry();
  return registry;
}

export function getScraper(provider) {
  const scraper = registry?.scraperMap?.[provider];
  if (!scraper) throw new ScraperError(400, `unsupported provider: ${provider}（启用：${ENABLED_PROVIDERS.join('/')}）`);
  return scraper;
}

export function assertProvider(provider) {
  if (!ENABLED_PROVIDERS.includes(provider)) {
    throw new ScraperError(400, `unsupported provider: ${provider}（启用：${ENABLED_PROVIDERS.join('/')}）`);
  }
}

export function toIdString(provider, resourceId) {
  return `${ID_PREFIX[provider]}${resourceId}`;
}

/** 集列表（含 preview/bonus/main 全量），可选按集号/期名匹配当前集。
 *
 * 腾讯特殊处理（实测踩坑）：上游 getEpisodes 在未传 episodeName 时，
 * selectEpisodeCandidates 会把 episodePart 强制过滤为 "whole"（正片"第1期上/中/下"
 * part=upper/middle/lower 被滤掉只剩陪看）；传 episodeName 又走 curated 精选分支
 * （只返回陪看/回放，漏正片）。因此腾讯直接调 getEpisodesPage 分页拿全量原始映射，
 * 匹配逻辑由本壳完成（正片优先 edition=main，支持"第X期[上下中]"解析）。
 *
 * getEpisodesPage 在上游 TS 层是 private（dist 运行时仍可调）；src 快照已打本地补丁
 * 显式公开。上游升级重建后若该方法消失（改 #private / 改名），下面先 fail-fast，
 * 不会静默退化成"无匹配集 → 无弹幕"。 */
export async function listEpisodes(provider, resourceId, { episode, episodeName, signal } = {}) {
  const scraper = getScraper(provider);
  let eps;
  if (provider === 'tencent') {
    if (typeof scraper.getEpisodesPage !== 'function') {
      throw new ScraperError(
        500,
        'vendored scraper-kit 缺少 getEpisodesPage：升级上游后需重新应用本地补丁并 rebuild dist（见 deploy/danmu/README.md 升级流程）',
      );
    }
    eps = [];
    // fullPage = 首页实返条数，即事实上的每页大小（不耦合上游 pageSize 常量：
    // 上游把 30 改成 20 时，旧写法 "length < 30" 会在第一页就停，少拉一整页）
    let fullPage = 0;
    for (let page = 0; page < 10; page += 1) {
      abortIf(signal);
      let pageEps;
      try {
        // eslint-disable-next-line no-await-in-loop
        pageEps = await scraper.getEpisodesPage(resourceId, page);
      } catch {
        break; // 腾讯超范围页返回 {ret:0,msg:''}（无 data），schema 校验抛错 → 停止翻页
      }
      if (!pageEps?.length) break;
      if (page === 0) fullPage = pageEps.length;
      eps.push(...pageEps);
      // 短页 = 最后一页（与首页比，不猜上游 pageSize）；总集数不足一页时会多一次
      // 抛错的探边请求，与"整页到底后抛错"同构，可接受
      if (pageEps.length < fullPage) break;
    }
  } else {
    abortIf(signal);
    eps = await scraper.getEpisodes(
      toIdString(provider, resourceId),
      episode !== undefined && episode !== '' ? Number(episode) : undefined,
    );
  }

  // 匹配（正片优先 edition=main）
  const mainCandidates = eps.filter((e) => (e.episodeEdition ?? 'main') === 'main');
  let matched = null;
  if (episodeName) {
    matched = eps.find((e) => e.episodeTitle === String(episodeName)) ?? null;
    if (!matched) {
      const m = String(episodeName).match(/^第\s*([0-9]+)\s*期(?:[:：])?(上|下|中)?/);
      if (m) {
        const n = Number(m[1]);
        const part = { 上: 'upper', 下: 'lower', 中: 'middle' }[m[2]];
        const cands = mainCandidates.filter((e) => e.episodeNumber === n && (!part || e.episodePart === part));
        matched = cands[0] ?? mainCandidates.find((e) => e.episodeNumber === n) ?? null;
      }
    }
  } else if (episode !== undefined && episode !== '') {
    const n = Number(episode);
    const cands = mainCandidates.filter((e) => e.episodeNumber === n);
    matched = cands[0] ?? null;
  }

  return { provider, episodes: eps, matched };
}

export async function listSegments(provider, episodeId, { signal } = {}) {
  const scraper = getScraper(provider);
  abortIf(signal);
  return scraper.getSegments(episodeId);
}

/**
 * 弹幕内容。
 * - segmentId 传入 → 单段点播（路径B：腾讯等）
 * - segmentId 缺省 → 整集聚合（路径A：B站，29 段串行，段间 ≥200ms）
 * 输出统一契约 { text, time(ms), color, mode }（mode: 1滚动/4底部/5顶部）
 */
export async function fetchComments(provider, episodeId, segmentId, { signal } = {}) {
  const scraper = getScraper(provider);
  const segments = segmentId
    ? [{ segmentId }]
    : await scraper.getSegments(episodeId);

  if (!segments || segments.length === 0) return [];

  const out = [];
  for (const seg of segments) {
    abortIf(signal);
    const items = await scraper.getComments(episodeId, seg.segmentId);
    if (items) {
      for (const it of items) {
        if (!it) continue;
        out.push({
          text: it.content,
          time: Math.round(it.timestamp * 1000), // 秒→毫秒（壳内唯一转换点）
          color: it.color ?? 0xffffff,
          mode: it.mode ?? 1,
        });
      }
    }
    if (seg !== segments[segments.length - 1]) await sleep(SEGMENT_GAP_MS);
  }
  return out;
}

/** 单 provider 并发 1（对外抓取串行，避免平台限流） */
const providerQueues = new Map();
export function withProviderLock(provider, fn) {
  const prev = providerQueues.get(provider) ?? Promise.resolve();
  const next = prev.then(fn, fn);
  providerQueues.set(
    provider,
    next.catch(() => {}),
  );
  return next;
}

export class ScraperError extends Error {
  constructor(status, message) {
    super(message);
    this.status = status;
  }
}

function abortIf(signal) {
  if (signal?.aborted) throw new ScraperError(504, 'request aborted');
}

function sleep(ms) {
  return new Promise((r) => setTimeout(r, ms));
}
