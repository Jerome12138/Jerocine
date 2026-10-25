/**
 * 弹幕服务 HTTP 适配器（scraper-kit 运行时契约，替代 @rexnow/libs-fetch 的 Fetch）。
 *
 * 契约：Fetch 把 restOptions（含 base64Data、params 等）原样透传给 adapter；
 * adapter 返回 { data, statusCode, headers }。Scraper 还会用到的能力：
 * - `cookie`（对象，可直接赋值清空）、`setCookie({k:v})`、`getCookie(name)`
 * - `headers`（默认请求头）、`setHeaders({k:v})`
 * - get/post 自动带 cookie jar 并吸收响应 Set-Cookie（优酷 token 链依赖：
 *   ensureTokenCookie 先访问 mmstat/acs 种 cna/_m_h5_tk，再签名拉弹幕）
 *
 * 关键：
 * - B站 seg.so protobuf 通道带 `base64Data: true`（options.base64Data），
 *   此时 data 必须返回 **base64 字符串**（上游用 base64ToUint8Array 解码）。
 * - 爱奇艺 cmts .z 弹幕为 **zlib 流**（magic 0x78）且响应**无 Content-Encoding 头**
 *   （Content-Type: application/octet-stream），Node fetch 不会自动解压；
 *   上游 zlibMode 只是标志、无解压实现 → 解压由壳在 adapter 完成（见 readResponse）。
 *   只测腾讯 JSON 路径的适配器会在这里漏掉 → 弹幕 XML 变乱码、fast-xml-parser
 *   报 "Maximum nested tags exceeded"（集成方案 §8.1 同类坑）。
 */
import zlib from 'node:zlib';

export function createHttpAdapter({ fetchImpl = globalThis.fetch } = {}) {
  const ctx = {
    cookie: {}, // 上游可能整体赋值清空（youku ensureTokenCookie: this.fetch.cookie = {}）
    headers: {},
  };
  return {
    cookie: ctx.cookie,
    headers: ctx.headers,
    setCookie(pairs) {
      Object.assign(ctx.cookie, pairs);
    },
    getCookie(name) {
      return ctx.cookie[name] ?? undefined;
    },
    setHeaders(pairs) {
      Object.assign(ctx.headers, pairs);
    },
    async get(url, options = {}) {
      const res = await fetchImpl(url, {
        headers: buildHeaders(ctx, options.headers),
        signal: options.signal,
      });
      absorbSetCookie(ctx, res.headers);
      return readResponse(res, options);
    },
    async post(url, body, options = {}) {
      const res = await fetchImpl(url, {
        method: 'POST',
        headers: buildHeaders(ctx, { 'Content-Type': 'application/json', ...(options.headers || {}) }),
        body: typeof body === 'string' ? body : JSON.stringify(body),
        signal: options.signal,
      });
      absorbSetCookie(ctx, res.headers);
      return readResponse(res, options);
    },
  };
}

/** 请求头合并顺序：默认头 → cookie jar → 显式 headers（显式 Cookie 覆盖 jar） */
function buildHeaders(ctx, explicit) {
  const out = { ...ctx.headers };
  const jarCookie = Object.entries(ctx.cookie)
    .map(([k, v]) => `${k}=${v}`)
    .join('; ');
  if (jarCookie) out.Cookie = jarCookie;
  return { ...out, ...(explicit || {}) };
}

/** 吸收 Set-Cookie（简版 RFC6265：只记 name=value；过期/删除则移除） */
function absorbSetCookie(ctx, headers) {
  let pairs;
  if (typeof headers.getSetCookie === 'function') {
    pairs = headers.getSetCookie();
  } else {
    const raw = headers.get('set-cookie');
    pairs = raw ? [raw] : [];
  }
  for (const pair of pairs) {
    const [kv, ...rest] = pair.split(';');
    const eq = kv.indexOf('=');
    if (eq <= 0) continue;
    const name = kv.slice(0, eq).trim();
    const value = kv.slice(eq + 1).trim();
    const isExpired = rest.some((part) => {
      const p = part.trim().toLowerCase();
      return p.startsWith('expires=') && new Date(part.split('=')[1]).getTime() < Date.now();
    });
    if (isExpired) delete ctx.cookie[name];
    else ctx.cookie[name] = value;
  }
}

async function readResponse(res, options) {
  const statusCode = res.status;
  const headers = {};
  for (const [k, v] of res.headers.entries()) {
    if (k === 'set-cookie') {
      // undici entries() 对多值 set-cookie 返回多条 → 合并为逗号分隔串
      // （@rexnow/libs-fetch handleResponse 按 split(",") 解析；丢失任意一条
      //   会导致优酷收 _m_h5_tk_enc 却收不到 _m_h5_tk → 签名 token 空）
      headers[k] = headers[k] ? `${headers[k]}, ${v}` : v;
    } else {
      headers[k] = v;
    }
  }

  const buf = Buffer.from(await res.arrayBuffer());

  if (options?.base64Data === true) {
    // B站 seg.so：返回 base64 字符串（Protobuf 二进制通道，不解压）
    return { data: buf.toString('base64'), statusCode, headers };
  }

  // 智能解压：gzip(1F 8B) / zlib(0x78 + {01,5E,9C,DA}) 且未声明 Content-Encoding 的流。
  // 解压失败按原文（上游自会容错），避免误伤普通文本响应。
  let text;
  if (isGzip(buf)) {
    text = safeInflate(buf, (b) => zlib.gunzipSync(b));
  } else if (isZlib(buf)) {
    text = safeInflate(buf, (b) => zlib.inflateSync(b));
  } else {
    text = buf.toString('utf8');
  }

  let data;
  try {
    data = JSON.parse(text);
  } catch {
    data = text;
  }
  return { data, statusCode, headers };
}

function isGzip(buf) {
  return buf.length >= 2 && buf[0] === 0x1f && buf[1] === 0x8b;
}

/** zlib magic：CMF 0x78 + FLG ∈ {01(最佳), 5E(默认), 9C(最快), DA(最快+字典)} */
function isZlib(buf) {
  return buf.length >= 2 && buf[0] === 0x78 && [0x01, 0x5e, 0x9c, 0xda].includes(buf[1]);
}

function safeInflate(buf, inflate) {
  try {
    return inflate(buf).toString('utf8');
  } catch {
    return buf.toString('utf8');
  }
}

/** 进程内 LRU 存储适配器（scraper-kit 段缓存 TTL 由其自身控制） */
export function createMemoryStorageAdapter({ maxEntries = 500 } = {}) {
  const map = new Map();
  return {
    async getJson(key) {
      const v = map.get(key);
      return v === undefined ? null : v;
    },
    setJson(key, value) {
      if (map.size >= maxEntries) {
        const oldest = map.keys().next().value;
        if (oldest !== undefined) map.delete(oldest);
      }
      map.set(key, value);
    },
  };
}
