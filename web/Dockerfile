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
# build = vue-tsc(类型强检验) + vite build; 重构后契约对齐, 生产构建强制类型门禁
RUN pnpm run build

# 运行镜像: nginx 静态托管 + /api/ 反代到 server.
FROM nginx:1.27-alpine

# nginx.conf 走仓库版本, 不再依赖宿主机 /opt/film/data/nginx 挂载
COPY deploy/data/nginx/nginx.conf /etc/nginx/nginx.conf
COPY --from=build /app/dist /usr/share/nginx/html

EXPOSE 80

# nginx 镜像自带 wget, healthcheck 直接探 /
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1/ >/dev/null 2>&1 || exit 1
