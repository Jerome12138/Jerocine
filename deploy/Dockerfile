# 后端镜像: golang:1.21-alpine 编译 → distroless 运行, 非 root, 无 shell.
# 上下文 = 仓库根 (见 docker-compose.yml build.context: ..)
# 入口为重构后的组合根 ./cmd/server。
FROM --platform=$BUILDPLATFORM golang:1.21-alpine AS build

ARG TARGETARCH
ENV GO111MODULE=on \
    CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=$TARGETARCH \
    GOPROXY=https://goproxy.cn,direct

WORKDIR /src

# 先拷依赖再拷代码, 利用 layer 缓存
COPY server/go.mod server/go.sum ./
RUN go mod download

COPY server/ ./
RUN go build -trimpath -ldflags="-s -w" -o /out/main ./cmd/server && \
    mkdir -p /out/static/upload

# 运行镜像: 静态 distroless, 非 root.
FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="jerocine-server"

WORKDIR /opt/server
COPY --from=build --chown=nonroot:nonroot /out/main /opt/server/main
COPY --from=build --chown=nonroot:nonroot /out/static /opt/server/static

USER nonroot:nonroot
EXPOSE 3601

# distroless 无 curl/wget; 健康检查用二进制自带的 -healthcheck 子命令(compose healthcheck 调用)。
ENTRYPOINT ["/opt/server/main"]
