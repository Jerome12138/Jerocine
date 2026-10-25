/**
 * 弹幕服务 HTTP/Storage 适配器（scraper-kit 运行时契约）。
 *
 * 契约：Fetch 把 restOptions（含 base64Data、params 等）原样透传给 adapter；
 * adapter 返回 { data, statusCode, headers }。
 *
 * 关键：B站 seg.so protobuf 通道带 `base64Data: true`（options.base64Data），
 * 此时 data 必须返回 **base64 字符串**（上游用 base64ToUint8Array 解码）。
 * 只测腾讯 JSON 路径的适配器会在这里漏掉 → protobuf 报 illegal tag（见集成方案 §8.1）。
 */

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
  const headers = Object.fromEntries(res.headers.entries());

  if (options?.base64Data === true) {
    // B站 seg.so：返回 base64 字符串（Protobuf 二进制通道）
    const buf = Buffer.from(await res.arrayBuffer());
    return { data: buf.toString('base64'), statusCode, headers };
  }

  const text = await res.text();
  let data;
  try {
    data = JSON.parse(text);
  } catch {
    data = text;
  }
  return { data, statusCode, headers };
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
