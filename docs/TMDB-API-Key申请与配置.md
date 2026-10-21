# TMDB API Key 申请与配置（横图回填功能）

本站首页轮播的横版大图（backdrop）来自 [TMDB](https://www.themoviedb.org/)（The Movie Database）。
后端按**片名 + 年份**在 TMDB 检索影片、下载横图到本地存储并回填数据库，前台与管理页直接使用本地图。

关键特性：

- **只对首页轮播影片回填**（手动配置位 + 热榜自动补位，共前 5 位），TMDB 用量与轮播规模成正比；
- **图片落本地 blob**（`/api/upload/backdrop/{mid}.jpg`），访问终端无需直连 TMDB（`image.tmdb.org` 在大陆不可直连）；
- **Key 走管理后台维护**（保存前验真、热生效、掩码显示），也可用服务器环境变量兜底；
- **未配置 Key = 功能整体关闭**，其余功能不受任何影响。

## 一、申请 API Key（免费）

1. 注册/登录 [www.themoviedb.org](https://www.themoviedb.org/signup) 账号；
2. 进入 **设置（Settings）→ API** → **申请 API Key（Click here to get an API key）**；
3. 用途选择 **Developer（开发者）**，用途说明随意填写（如 personal hobby project），提交即生效；
4. 在 API 详情页会看到两组凭据，**任选其一**（后端自动识别）：
   - **API Key（v3 auth）**：32 位十六进制字符串，如 `001122aabbccddeeff001122aabbccdd`（仅为格式示例，请勿照抄）—— **推荐，短且好粘贴**；
   - **API Read Access Token（v4 auth）**：三段式长 JWT（`eyJhbGciOi...`），走 Bearer 头。

> 免费档限速约 50 req/s，本站 worker 自限约 3 req/s，正常使用不会触顶。
> Key 丢失可随时回到同一页面找回，无需备份明文。

## 二、配置

### 方式 A（唯一）：管理后台配置，即时生效

登录管理后台 → **系统 → 系统设置 → TMDB 横图**，粘贴 v3 key 或 v4 Read Access Token 后点「保存 Key」：

- 保存前服务端会到 TMDB 验真，无效 key 直接拒绝，避免横图静默不回填；
- 保存即热生效（worker 下一轮立即用新 key），**无需重启**；已配置的 key 在后台只显示中间脱敏掩码
  （如 `0c06…676b`，规则与 SMTP 授权码一致），明文不可回看；
- 「清除」即停用横图回填，已落地的图片不受影响。

> **凭据只存数据库，后台是唯一权威**（2026-10-21 起 `TMDB_API_KEY` 环境变量已移除）。
> **升级注意**：如果你的 key 以前配在 `deploy/.env` 的 `TMDB_API_KEY` 里，升级后数据库里仍是空的，
> 后台会显示「未配置」且横图回填会停 —— 到上面这个页面重新粘贴保存一次即可，之后 `.env` 里那行可以删掉。

### 仍然保留的 env（非凭据，基本不用改）

```bash
# 检索结果语言，默认 zh-CN（中文标题/中文简介匹配更准）
TMDB_LANG=zh-CN
# 自建 API 反代时才需要（默认官方 https://api.themoviedb.org）
TMDB_API_BASE=
# 自建图片反代时才需要（默认官方 https://image.tmdb.org）
TMDB_IMAGE_BASE=
```

改这几项属于启动期配置，改完需要重建 server 容器（纯前端更新不用）：
```bash
cd /path/to/Jerocine/deploy
docker compose --env-file .env up -d --no-deps server
```

## 三、验证是否生效

1. **服务日志**：worker 启动即跑一轮；检索无果只记一行日志，不打扰主流程：
   ```bash
   docker logs jerocine_server --since 5m 2>&1 | grep -i backdrop
   ```
2. **管理页**：后台 → 首页轮播，生效位的「横图已就绪 / 横图待回填」标签即回填进度；
3. **图片落盘**（本地 blob）：
   ```bash
   ls /var/lib/docker/volumes/jerocine_uploads/_data/backdrop/   # 宿主机
   curl -I https://你的域名/api/upload/backdrop/<mid>.jpg          # 访问（应 200）
   ```

## 四、行为细则

| 行为 | 说明 |
|---|---|
| 触发时机 | worker 启动即跑一轮；此后**采集落库 / 后台改轮播**立即触发（事件驱动），另有 20 分钟兜底扫描 |
| 检索策略 | 片名(+年份) → TMDB `search/movie` 与 `search/tv` 双路；年份错标时自动降级宽松匹配 |
| 检索无果 | 影片标记 `-` 哨兵，不再重复查询；网络失败不标记，下一轮自动重试 |
| 采集安全 | 横图列被排除在采集 upsert 之外，源站重推数据不会抹掉已回填的图；全量重采换表前会回灌 |
