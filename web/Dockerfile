# 多阶段: node 构建前端 → nginx 静态托管.
# 上下文 = 仓库根 (compose build.context: ..)
FROM node:20-alpine AS build

WORKDIR /app

# 启用 corepack 并固定 pnpm 9 (pnpm 11 要求 node>=22, 当前 base 是 node:20)
RUN corepack enable && corepack prepare pnpm@9 --activate

# 先拷锁文件再拷源码, 复用 docker layer 缓存
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY web/ ./

# 版本目录(方案 §3.1, 构建期注入): 留空 ⇒ 产物仍落 assets/(本地开发/CI 不受影响);
# 部署脚本传 =YYYYMMDD-HHMMSS ⇒ 产物落 assets/<TS>/, index.html 引用天然带版本目录,
# Workbox 在同一次 build 里 glob 出的 precache manifest 与线上 URL 自洽(无部署期改写)。
ARG JC_BUILD_TS=""
ENV JC_BUILD_TS=${JC_BUILD_TS}

# build = vue-tsc(类型强检验) + vite build; 重构后契约对齐, 生产构建强制类型门禁
RUN pnpm run build

# 运行镜像: nginx 静态托管 + /api/ 反代到 server.
FROM nginx:1.27-alpine

# nginx.conf 走仓库版本, 不再依赖宿主机 /opt/film/data/nginx 挂载
COPY deploy/data/nginx/nginx.conf /etc/nginx/nginx.conf

# 注意: 这里**不再** COPY dist / LICENSE 进镜像 —— 静态目录改由宿主机挂载
# (compose: ./data/html:/usr/share/nginx/html), 打进镜像只会被挂载点遮蔽,
# 且会毁掉"保留 3 版 / 整版回滚"的前提(镜像重建即全丢)。产物与 LICENSE 由
# deploy/lib/web-release.sh 同步进挂载目录(见 deploy.sh web)。
# 因没有任何 stage 再依赖 build, `docker build` 本文件时会**跳过 node 构建阶段**(很快);
# 需要产物时用 `--target build` 单独构建复用该阶段(见 deploy.sh web)。

EXPOSE 80

# nginx 镜像自带 wget, healthcheck 直接探 /
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1/ >/dev/null 2>&1 || exit 1
