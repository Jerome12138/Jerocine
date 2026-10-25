/**
 * 弹幕服务 HTTP 适配器（scraper-kit 运行时契约，替代 @rexnow/libs-fetch 默认 adapter）。
 *
 * 契约（上游 packages/fetch/src/index.ts 实证）：
 * - Fetch.get/post 先经实例 buildHeaders（实例 headers + 实例 cookie + 调用方
 *   headers 三层合并）再整体传给 adapter —— **adapter 收到的 options.headers
 *   即最终请求头，原样透传即可**。
 * - cookie 生命周期全在上游 Fetch 实例层（handleResponse 吸收 Set-Cookie →
 *   实例 setCookie；优酷 ensureTokenCookie 的 `this.fetch.cookie = {}` 清的是
 *   实例字段，下轮 buildHeaders 自然不带旧值）。每个 scraper 独立 new Fetch
 *   （createScraperRegistry），平台间 cookie 天然隔离。
 *   ⚠️ adapter 层**不做** cookie jar：单例 adapter + 多 Fetch 实例的架构下，
 *   jar 会把 A 平台的 cookie 透传给 B 平台域名（跨站泄漏），且清空语义
 *   （this.fetch.cookie = {}）在 adapter 层无法真正生效。
 * - adapter 返回 { data, statusCode, headers }；Set-Cookie 多值必须合并为
 *   逗号分隔串（上游 handleResponse 按 split(",") 解析，丢任意一条会导致
 *   优酷收到 _m_h5_tk_enc 却收不到 _m_h5_tk → 签名 token 空）。
 *
 * 关键（集成方案 §8.1 两类坑）：
 * - B站 seg.so protobuf 通道带 `base64Data: true`（options.base64Data），
 *   此时 data 必须返回 **base64 字符串**（上游用 base64ToUint8Array 解码）。
 * - 爱奇艺 cmts .z 弹幕为 **zlib 流**（magic 0x78）且响应**无 Content-Encoding
 *   头**（Content-Type: application/octet-stream），Node fetch 不会自动解压；
 *   上游 zlibMode 只是标志、无解压实现 → 解压由壳在 readResponse 完成。
 */
import zlib from 'node:zlib';

export function createHttpAdapter({ fetchImpl = globalThis.fetch } = {}) {
  return {
    async get(url, options = {}) {
      const res = await fetchImpl(url, {
        headers: options.headers,
        signal: options.signal,
      });
      return readResponse(res, options);
    },
    async post(url, body, options = {}) {
      const res = await fetchImpl(url, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', ...(options.headers || {}) },
        body: typeof body === 'string' ? body : JSON.stringify(body),
        signal: options.signal,
      });
      return readResponse(res, options);
    },
  };
}

async function readResponse(res, options) {
  const statusCode = res.status;
  const headers = {};
  for (const [k, v] of res.headers.entries()) {
    if (k === 'set-cookie') {
      // undici entries() 对多值 set-cookie 返回多条 → 合并为逗号分隔串
      // （@rexnow/libs-fetch handleResponse 按 split(",") 解析）
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
