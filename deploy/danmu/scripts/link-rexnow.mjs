/**
 * 将上游 workspace 包（@rexnow/*，不在 npm registry）从快照链接到 node_modules。
 * 部署构建时随 npm install（postinstall）自动执行；本地开发手动 `node scripts/link-rexnow.mjs`。
 * 4 个包均只使用已构建的 dist 产物（编译不上生产机）。
 */
import { mkdirSync, cpSync, existsSync } from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const nm = path.join(root, 'node_modules');
const rexnow = path.join(nm, '@rexnow');
mkdirSync(rexnow, { recursive: true });

const PACKAGES = [
  ['utils', '@rexnow/libs-utils'],
  ['storage', '@rexnow/libs-storage'],
  ['fetch', '@rexnow/libs-fetch'],
  ['scraper-kit', '@rexnow/scraper-kit'],
];

for (const [dir, pkgName] of PACKAGES) {
  const src = path.join(root, 'upstream', 'packages', dir);
  const dest = path.join(rexnow, pkgName.replace('@rexnow/', ''));
  if (!existsSync(src)) {
    console.error(`[link-rexnow] 缺少快照: ${src}`);
    process.exit(1);
  }
  cpSync(src, dest, { recursive: true });
  console.log(`[link-rexnow] linked ${pkgName} -> ${dest}`);
}
