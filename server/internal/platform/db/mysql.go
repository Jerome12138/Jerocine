package db

import (
	"log"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const (
	mysqlMaxOpenConns    = 100
	mysqlMaxIdleConns    = 20
	mysqlConnMaxLifetime = 30 * time.Minute
	mysqlRetryAttempts   = 5
	mysqlRetryBase       = time.Second
)

// InitMySQL 打开 gorm 连接, 配置连接池, 指数退避重试。
// 表结构由 migrations 管理(golang-migrate), 这里不做 AutoMigrate。
func InitMySQL(dsn string) (*gorm.DB, error) {
	var lastErr error
	for i := 0; i < mysqlRetryAttempts; i++ {
		gdb, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
			Logger:                 logger.Default.LogMode(logger.Warn),
			SkipDefaultTransaction: true, // 单条写不自动包事务, 显式事务走 TxManager
		})
		if err == nil {
			sqlDB, e := gdb.DB()
			if e == nil {
				if pingErr := sqlDB.Ping(); pingErr == nil {
					sqlDB.SetMaxOpenConns(mysqlMaxOpenConns)
					sqlDB.SetMaxIdleConns(mysqlMaxIdleConns)
					sqlDB.SetConnMaxLifetime(mysqlConnMaxLifetime)
					return gdb, nil
				} else {
					lastErr = pingErr
				}
			} else {
				lastErr = e
			}
		} else {
			lastErr = err
		}
		wait := mysqlRetryBase << i
		log.Printf("WARN mysql: 连接失败 (%d/%d): %v, %s 后重试", i+1, mysqlRetryAttempts, lastErr, wait)
		time.Sleep(wait)
	}
	return nil, lastErr
}

// InitMySQLNoBinlog 打开一个**会话级不写 binlog**（`sql_log_bin=0`）的 gorm 连接。
//
// 用途（2026-11-13）：**只给"可重建的派生数据"的写入用** —— 即 `movie_episode_epid` 的
// 整源行 DELETE+INSERT、孤儿清理、`TRUNCATE` 全量重建、以及派生存量计数器维护。
// 理由：派生表是**单向派生产物**，丢 binlog 也能由 `movie_play_source` 重算回来，不记 binlog 是安全的；
// 而它的写入量最大（全量 rebuild 一轮就是 650 万行级的 DELETE+INSERT），是 binlog 的主要来源之一。
//
// ⚠️ **绝不可**用于真相源（`movie_play_source` / `movie` / `users` / `movie_epid_override` …）——
// 那些必须保住 PITR（时间点恢复）能力。
// ⚠️ 打不开时**由调用方降级**为主连接（照常写 binlog）：本特性只优化 binlog 体积，不该阻断派生。
func InitMySQLNoBinlog(dsn string) (*gorm.DB, error) {
	return InitMySQL(withSessionVar(dsn, "sql_log_bin=0"))
}

// withSessionVar 在 DSN 上追加一个**会话变量**参数。
// go-sql-driver 会把不认识的参数当作系统变量，在**每条连接建立时**执行一次 `SET <k>=<v>` ✓
// （连接池下每条新连接都会设，语义正确）。兼容"已有 query"与"无 query"两种形态。
func withSessionVar(dsn, kv string) string {
	if dsn == "" || kv == "" {
		return dsn
	}
	if strings.Contains(dsn, "?") {
		return dsn + "&" + kv
	}
	return dsn + "?" + kv
}
