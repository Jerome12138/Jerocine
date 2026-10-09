#!/usr/bin/env node
/**
 * 构建产物布局断言 (离线可跑, 不依赖网络 / 浏览器).
 *
 * 校验目标:
 *   1. dist/index.html 里引用的每个 /assets/... 资源都能在 dist/ 下找到实体文件
 *      (防"引用路径与产物不一致"这类静默 404)。
 *   2. 注入 JC_BUILD_TS 时, 资源确实落在 dist/assets/<TS>/ 下 (方案 A 的版本目录)。
 *   3. index.html 不引用任何 .map 文件 (sourcemap 不上线)。
 *   4. (若已接入 PWA) dist 根存在 sw.js 与 workbox-*.js。
 *
 * 用法:
 *   node scripts/verify-build-layout.mjs                 # 普通构建后校验
 *   JC_BUILD_TS=20261009-190000 node scripts/verify-build-layout.mjs
 */
import { readFileSync, existsSync, readdirSync } from 'node:fs'
import { resolve, dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

const here = dirname(fileURLToPath(import.meta.url))
const webRoot = resolve(here, '..')
const distDir = resolve(webRoot, 'dist')
const indexHtml = resolve(distDir, 'index.html')

const errors = []
const notes = []
const ok = (m) => console.log(`  ✓ ${m}`)
const fail = (m) => {
  errors.push(m)
  console.log(`  ✗ ${m}`)
}

console.log(`[verify-build-layout] dist = ${distDir}`)

if (!existsSync(indexHtml)) {
  console.error(`[verify-build-layout] 找不到 ${indexHtml}, 请先执行构建`)
  process.exit(1)
}

// ---- 1/3: 解析 index.html 的资源引用 ----
const html = readFileSync(indexHtml, 'utf8')
/** 站内绝对路径引用 (排除 // 协议相对 与外部 URL / /api 接口) */
const localRefs = new Set()
const re = /(?:src|href)="(\/[^"]*)"/g
let m
while ((m = re.exec(html)) !== null) {
  const ref = m[1]
  if (ref.startsWith('//') || ref.startsWith('/api/')) continue
  localRefs.add(ref)
}
const assetRefs = new Set([...localRefs].filter((r) => r.startsWith('/assets/')))

if (assetRefs.size === 0) fail('index.html 里没有任何 /assets/ 引用 (构建可能异常)')
else ok(`index.html 引用 ${assetRefs.size} 个 /assets/ 资源`)

for (const ref of localRefs) {
  const target = join(distDir, ref.replace(/^\//, ''))
  if (!existsSync(target)) fail(`引用缺失: ${ref}`)
  if (ref.endsWith('.map')) fail(`index.html 引用了 sourcemap: ${ref}`)
}
if (localRefs.size > 0 && errors.length === 0)
  ok(`全部 ${localRefs.size} 个站内引用均可解析, 且无 .map 引用`)

// ---- 2: JC_BUILD_TS 版本目录 ----
const ts = process.env.JC_BUILD_TS
if (ts) {
  const tsDir = resolve(distDir, 'assets', ts)
  if (!existsSync(tsDir)) {
    fail(`JC_BUILD_TS=${ts} 但 dist/assets/${ts}/ 不存在`)
  } else {
    for (const ref of assetRefs) {
      if (!ref.startsWith(`/assets/${ts}/`)) {
        fail(`引用未落在版本目录下: ${ref} (期望 /assets/${ts}/...)`)
      }
    }
    if (errors.length === 0) ok(`资源已落在版本目录 assets/${ts}/ 且引用一致`)
    if (existsSync(join(tsDir, 'index.html'))) {
      notes.push('注意: 版本目录内存在 index.html (不应出现)')
    }
  }
} else {
  const flat = readdirSync(resolve(distDir, 'assets'), { withFileTypes: true })
    .filter((d) => d.isDirectory())
    .map((d) => d.name)
  if (flat.length > 0) notes.push(`未注入 JC_BUILD_TS, 但 assets/ 下有子目录: ${flat.join(', ')}`)
  ok('未注入 JC_BUILD_TS (普通构建布局)')
}

// ---- 4: PWA 产物 (可选) ----
const rootFiles = readdirSync(distDir)
const hasSw = rootFiles.includes('sw.js')
const workbox = rootFiles.filter((f) => /^workbox-.*\.js$/.test(f))
if (hasSw) {
  ok('sw.js 存在')
  if (workbox.length === 0) notes.push('sw.js 存在但未见 workbox-*.js (检查 workbox 配置)')
  else ok(`workbox 运行时存在: ${workbox.join(', ')}`)
  // precache manifest 必须与真实 URL 一致: 若注入了 TS, sw.js 里应出现 assets/<TS>/
  if (ts) {
    const sw = readFileSync(resolve(distDir, 'sw.js'), 'utf8')
    if (sw.includes(`assets/${ts}/`)) ok(`sw.js precache manifest 含版本目录 assets/${ts}/ (自洽)`)
    else fail(`sw.js precache manifest 未含 assets/${ts}/ —— 部署后 precache 会全部 404`)
  }
} else {
  notes.push('未发现 sw.js (尚未接入 PWA 时可忽略)')
}

// 根目录里除 sw.js / workbox-*.js 之外的无 hash JS(如 registerSW.js)需单独处理缓存;
// 走 injectRegister:'inline' 时不应出现。
const strayRootJs = rootFiles.filter(
  (f) => f.endsWith('.js') && !/^workbox-.*\.js$/.test(f) && f !== 'sw.js'
)
if (strayRootJs.length) {
  notes.push(`根目录存在无 hash 的 JS(需 no-cache 规则或改内联): ${strayRootJs.join(', ')}`)
}

// ---- 汇总 ----
if (notes.length) {
  console.log('\n提示:')
  for (const n of notes) console.log(`  - ${n}`)
}
if (errors.length) {
  console.error(`\n[verify-build-layout] 失败: ${errors.length} 项`)
  process.exit(1)
}
console.log('\n[verify-build-layout] 通过')
