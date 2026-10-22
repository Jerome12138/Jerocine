# nginx **运行镜像**(树内版): 只放仓库版 nginx.conf, 静态产物走宿主机挂载(不进镜像)。
#
# 谁在用: `./deploy.sh nginx`(树上运维) —— compose nginx 默认 build(context=仓库根)。
# 包内工具链模式用的是它的镜像 deploy/Dockerfile.nginx(随产物包 tooling/ 上送,
# 上下文 = deploy/runtime/), 两者运行 stage 逐行一致, 由
# deploy/tests/tooling-selfcontain.test.sh 断言(FROM / COPY 目标 / HEALTHCHECK)。
#
# ⚠️ 这里曾经还有 node 构建阶段(vite build, 服务于树上 `./deploy.sh web` 现编译) ——
# 2026-10-22 起该路径已删除: 前端构建只走 scripts/build-web.sh(本机 / CI / 应急容器),
# 编译不上生产机。文件因此从多阶段瘦身为单阶段。
FROM nginx:1.27-alpine

# nginx.conf 走仓库版本(上下文=仓库根, 故路径带 deploy/ 前缀)
COPY deploy/data/nginx/nginx.conf /etc/nginx/nginx.conf

# **不** COPY dist / LICENSE 进镜像 —— 静态目录由宿主机挂载
# (compose: ./data/html:/usr/share/nginx/html), 打进镜像只会被挂载点遮蔽, 且会毁掉
# "保留 3 版 / 整版回滚"的前提(镜像重建即全丢)。产物与 LICENSE 由
# deploy/lib/web-release.sh 同步进挂载目录(见 deploy.sh web-deploy)。

EXPOSE 80

# nginx 镜像自带 wget, healthcheck 直接探 /
HEALTHCHECK --interval=15s --timeout=3s --start-period=10s --retries=3 \
  CMD wget -qO- http://127.0.0.1/ >/dev/null 2>&1 || exit 1
