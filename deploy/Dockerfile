# 后端镜像: golang:1.27-alpine 编译 → distroless 运行, 非 root, 无 shell.
# 版本须与 server/go.mod 的 go 指令(1.27)一致: gin v1.12 起要求 go >= 1.25,
# golang.org/x/crypto v0.57 起要求 go >= 1.26。
# 上下文 = 仓库根 (见 docker-compose.yml build.context: ..)
# 入口为重构后的组合根 ./cmd/server。
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build

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
    mkdir -p /out/static/upload /out/data && cp -r data /out/data

# 运行镜像: 静态 distroless, 非 root.
FROM gcr.io/distroless/static-debian12:nonroot

LABEL org.opencontainers.image.title="jerocine-server"

WORKDIR /opt/server
COPY --from=build --chown=nonroot:nonroot /out/main /opt/server/main
COPY --from=build --chown=nonroot:nonroot /out/static /opt/server/static
# IP 归属地离线库(geoip; 缺失时在线明细归属地列留空, 不阻断启动)
COPY --from=build --chown=nonroot:nonroot /out/data /opt/server/data
# 许可随镜像分发(MIT + GoFilm 衍生声明)
COPY LICENSE /licenses/LICENSE

USER nonroot:nonroot
EXPOSE 3601

# distroless 无 curl/wget; 健康检查用二进制自带的 -healthcheck 子命令(compose healthcheck 调用)。
ENTRYPOINT ["/opt/server/main"]
