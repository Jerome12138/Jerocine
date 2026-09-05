// Package blobstore 抽象二进制对象存储(图片/APK)。
// 接口隔离: 当前 local(磁盘卷)实现, 终态可平滑换 OSS/S3 而不动 service/repository。
// 解决蓝图阻断项③: 旧代码二进制读写散落、无抽象。
package blobstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"server/internal/platform/safehttp"
)

// BlobStore 对象存储端口。objectKey 为相对路径(如 "poster/123.jpg"), URL() 给出可访问地址。
type BlobStore interface {
	Save(ctx context.Context, objectKey string, data []byte) (url string, err error)
	SaveFromURL(ctx context.Context, objectKey, srcURL string) (url string, err error)
	Delete(ctx context.Context, objectKey string) error
	URL(objectKey string) string
}

// localStore 把对象写到本地目录(docker 卷 film_uploads), URL = baseURL + objectKey。
type localStore struct {
	dir     string
	baseURL string
	client  *http.Client
}

// NewLocal 构造本地对象存储。dir 为落盘根目录, baseURL 为对外访问前缀。
func NewLocal(dir, baseURL string) BlobStore {
	return &localStore{
		dir:     dir,
		baseURL: strings.TrimRight(baseURL, "/") + "/",
		client:  safehttp.NewClient(30 * time.Second), // 防 SSRF: dial 期拦内网 IP + 重定向校验
	}
}

// objectKeyRe 严格白名单: 仅允许字母数字与 . _ / -。
var objectKeyRe = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)

// resolvePath 规范化 objectKey 并做包含性校验, 防路径穿越(黑名单 ".." 不可靠, 改用 canonical + 前缀校验)。
func (s *localStore) resolvePath(objectKey string) (string, error) {
	key := strings.ReplaceAll(objectKey, "\\", "/")
	if key == "" || strings.HasPrefix(key, "/") || !objectKeyRe.MatchString(key) {
		return "", errors.New("blobstore: invalid object key") // 仅接受相对 key
	}
	// 显式拒绝任何 ".." 段(防穿越; 配合后面的 canonical + 前缀包含校验双保险)。
	for _, seg := range strings.Split(key, "/") {
		if seg == ".." {
			return "", errors.New("blobstore: object key contains ..")
		}
	}
	clean := path.Clean("/" + key) // 折叠, 强制以 / 开头
	if clean == "/" || strings.Contains(clean, "..") {
		return "", errors.New("blobstore: invalid object key")
	}
	full := filepath.Join(s.dir, filepath.FromSlash(clean))
	absDir, err := filepath.Abs(s.dir)
	if err != nil {
		return "", err
	}
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	if absFull != absDir && !strings.HasPrefix(absFull, absDir+string(os.PathSeparator)) {
		return "", errors.New("blobstore: object key escapes base dir")
	}
	return absFull, nil
}

func (s *localStore) URL(objectKey string) string {
	clean := path.Clean("/" + strings.ReplaceAll(objectKey, "\\", "/"))
	return s.baseURL + strings.TrimLeft(clean, "/")
}

func (s *localStore) Save(ctx context.Context, objectKey string, data []byte) (string, error) {
	p, err := s.resolvePath(objectKey)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return "", err
	}
	return s.URL(objectKey), nil
}

func (s *localStore) SaveFromURL(ctx context.Context, objectKey, srcURL string) (string, error) {
	// 防 SSRF: 预校验协议/主机为公网(dial 期 Control 兜底 redirect/rebinding)。
	if _, err := safehttp.ValidateURL(srcURL); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srcURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("blobstore: fetch %s status %d", srcURL, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20)) // 32MB 上限
	if err != nil {
		return "", err
	}
	return s.Save(ctx, objectKey, data)
}

func (s *localStore) Delete(ctx context.Context, objectKey string) error {
	p, err := s.resolvePath(objectKey)
	if err != nil {
		return err
	}
	err = os.Remove(p)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
