package db

import (
	"log"
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
