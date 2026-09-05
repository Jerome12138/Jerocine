# migrations

版本化数据库迁移（golang-migrate 格式），替代旧的 GORM `AutoMigrate` + 运行期建索引。

## 约定
- 文件名：`{version}_{name}.{up|down}.sql`，version 六位零填充。
- 业务内容表（movie/movie_search/...）**禁软删除**：硬删 + `state` 业务状态；主键 `mid/id` 全局唯一、不嵌 cid。
- 时间戳：业务表 `created_at/updated_at` 用 `BIGINT` 毫秒；user/telemetry 系列用 `datetime(3)`。
- `db_score` 以 `DECIMAL(3,1)` 存储，出参转 `float64`（裸数字）。
- 全文检索用 `FULLTEXT ... WITH PARSER ngram`，需 MySQL 启动参数 `--ngram-token-size=2`。

## 版本
| 版本 | 内容 |
|---|---|
| 000001_core_content | movie / movie_search / movie_play_source / category |
| 000002_config_and_jobs | collect_source / cron_task / site_config / app_version / files |
| 000003_user_and_telemetry | users / user_history / user_favorite / telemetry_event / telemetry_issue_resolution |
| 000004_seed_sources_and_cron | site_config 单行兜底 + 默认 cron（初始停用） |

## 运行
迁移文件含多条语句，MySQL DSN 需带 `multiStatements=true`。

```bash
# 本地（已装 migrate CLI）
migrate -path server/migrations \
  -database "mysql://user:pass@tcp(127.0.0.1:3306)/FilmSite?multiStatements=true" up

# docker-compose（M6 中以一次性 migrate 服务运行，film 依赖其 completed）
docker compose run --rm migrate
```

回滚：`migrate ... down 1`（逐版本）。
