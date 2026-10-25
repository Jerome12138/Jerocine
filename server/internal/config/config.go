package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config 单一配置源, 启动期 Load() 一次性从环境变量加载并校验。
// 关键凭据缺失 → fail-fast(log.Fatal), 不再像旧代码散落 init() + dev 默认值掩盖。
type Config struct {
	ServerPort string
	MySQLDSN   string
	Redis      RedisConfig
	JWT        JWTConfig
	CORS       []string
	Spider     SpiderConfig
	Blob       BlobConfig
	TMDB       TMDBConfig
	Danmu      DanmuConfig
	PageSizes  PageSizes
}

type RedisConfig struct {
	Addr     string
	Password string
	CacheDB  int // db0: 缓存
	CoordDB  int // db1: 协调态(锁/令牌桶/job/暂存)
}

type JWTConfig struct {
	PrivateKey []byte
	PublicKey  []byte
	Issuer     string
	TTL        time.Duration
	MaxDevices int
}

type SpiderConfig struct {
	ResetToken string // 清库二次确认 token, 常量时间比较
}

type BlobConfig struct {
	Driver   string // local | oss
	LocalDir string
	BaseURL  string // 访问前缀, 如 /api/upload/pic/poster/
}

// TMDBConfig 横图(backdrop)回填配置。API key 只存 site_config(后台「系统设置」维护,
// 运行期热替换), 这里只剩"基本不会改"的语言与网络地址。
// 可选 TMDB_API_BASE 支持自建反代; 图片由 worker 下载到本地 blob, 终端不直连 TMDB。
type TMDBConfig struct {
	Lang      string // 检索语言, 默认 zh-CN
	APIBase   string // 默认 https://api.themoviedb.org/3
	ImageBase string // 默认 https://image.tmdb.org/t/p/
}

// DanmuConfig 弹幕数据服务(danmu-service, Node 壳)配置。
// 容器网络内走 compose 服务名; 本地联调用环境变量指向 127.0.0.1:3611。
type DanmuConfig struct {
	ServiceURL string // 默认 http://danmu:3611
}

// PageSizes 各接口默认页大小, 集中而非散落各 handler。
type PageSizes struct {
	Home     int
	Classify int
	Filter   int
	Search   int
	Gallery  int
}

// Load 加载配置。fatalOnMissing=true 时关键凭据缺失直接退出进程。
func Load() *Config {
	c := &Config{
		ServerPort: env("SERVER_PORT", "3601"),
		MySQLDSN:   mustEnv("MYSQL_DSN"),
		Redis: RedisConfig{
			Addr:     env("REDIS_ADDR", "127.0.0.1:6379"),
			Password: os.Getenv("REDIS_PASSWORD"),
			CacheDB:  envInt("REDIS_DB_CACHE", 0),
			CoordDB:  envInt("REDIS_DB_COORD", 1),
		},
		JWT: JWTConfig{
			PrivateKey: mustKey("JWT_PRIVATE_KEY", "JWT_PRIVATE_KEY_FILE"),
			PublicKey:  mustKey("JWT_PUBLIC_KEY", "JWT_PUBLIC_KEY_FILE"),
			Issuer:     env("JWT_ISSUER", "Jerocine"),
			// TTL / MaxDevices 只是构造期兜底: 实际生效值读 site_config(后台「账号与安全」),
			// 由 UserService.SetPolicyProvider 覆盖, 不再读环境变量。
			TTL:        168 * time.Hour,
			MaxDevices: 5,
		},
		CORS: splitNonEmpty(os.Getenv("CORS_ALLOWED_ORIGINS")),
		Spider: SpiderConfig{
			ResetToken: mustEnv("SPIDER_RESET_TOKEN"),
		},
		Blob: BlobConfig{
			Driver:   env("BLOB_DRIVER", "local"),
			LocalDir: env("BLOB_LOCAL_DIR", "./static/upload"),
			BaseURL:  env("BLOB_BASE_URL", "/api/upload/"),
		},
		TMDB: TMDBConfig{
			Lang:      env("TMDB_LANG", "zh-CN"),
			APIBase:   os.Getenv("TMDB_API_BASE"),
			ImageBase: os.Getenv("TMDB_IMAGE_BASE"),
		},
		Danmu: DanmuConfig{
			ServiceURL: env("DANMU_SERVICE_URL", "http://danmu:3611"),
		},
		PageSizes: PageSizes{Home: 14, Classify: 21, Filter: 49, Search: 10, Gallery: 39},
	}
	if c.Redis.CacheDB == c.Redis.CoordDB {
		log.Fatalf("config: REDIS_DB_CACHE 与 REDIS_DB_COORD 不能相同 (均为 %d)", c.Redis.CacheDB)
	}
	return c
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("config: 必填环境变量 %s 未设置", key)
	}
	return v
}

func envInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
		log.Printf("WARN config: %s=%q 解析失败, 回退 %d", key, v, def)
	}
	return def
}

// mustKey 二选一加载密钥: 优先内容直传 (contentEnv), 否则读文件 (fileEnv); 都没有则 fatal。
func mustKey(contentEnv, fileEnv string) []byte {
	if v := os.Getenv(contentEnv); v != "" {
		return []byte(v)
	}
	if f := os.Getenv(fileEnv); f != "" {
		b, err := os.ReadFile(f)
		if err != nil {
			log.Fatalf("config: 读取 %s=%q 失败: %v", fileEnv, f, err)
		}
		return b
	}
	log.Fatalf("config: 必须设置 %s 或 %s 之一", contentEnv, fileEnv)
	return nil
}

func splitNonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// String 脱敏摘要, 启动日志用 (不打印密钥/密码)。
func (c *Config) String() string {
	return fmt.Sprintf("Config{port=%s mysql=*** redis=%s(cache=%d,coord=%d) cors=%v blob=%s tmdb.lang=%s danmu=%s}",
		c.ServerPort, c.Redis.Addr, c.Redis.CacheDB, c.Redis.CoordDB, c.CORS, c.Blob.Driver, c.TMDB.Lang, c.Danmu.ServiceURL)
}
