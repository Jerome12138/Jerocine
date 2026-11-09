package service

import (
	"bytes"
	"context"
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- 横图(backdrop)回填单测 ----
//
// ⚠ 本文件**故意不 import image/jpeg / image/png**。
// image.Decode 的分发依赖各格式包在 init() 注册解码器; 若测试文件自己 import 了,
// 就会替被测包把解码器注册进测试二进制, 让"缺注册"的回归无法被发现。
// 素材以二进制文件放 testdata, 解码器注册状态完全由 service 包(backdrop_service.go)决定。

// capturingBlob 记录 Save 的 objectKey 与字节内容; 其余方法空实现(仅本用例需要 Save)。
type capturingBlob struct {
	key  string
	data []byte
}

func (b *capturingBlob) Save(_ context.Context, objectKey string, data []byte) (string, error) {
	b.key = objectKey
	b.data = append([]byte(nil), data...)
	return "http://blob.test/" + objectKey, nil
}

func (b *capturingBlob) SaveFromURL(context.Context, string, string) (string, error) {
	return "", nil
}

func (b *capturingBlob) Delete(context.Context, string) error { return nil }

func (b *capturingBlob) URL(objectKey string) string { return "http://blob.test/" + objectKey }

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("读素材 %s: %v", name, err)
	}
	return b
}

// TestSaveBackdropEncodesJPEGToWebP 钉住 TMDB 横图的 WebP 落盘: 喂 JPEG 原图必须
// 产出 .webp 键与 WebP 字节。
//
// 回归守卫: backdrop_service.go 若漏掉 `_ "image/jpeg"`, image.Decode 会解不了 JPEG
// (返回 image: unknown format) → 静默退回 .jpg 分支, WebP 转换形同虚设
// (2026-11-09 定位的线上现状)。
func TestSaveBackdropEncodesJPEGToWebP(t *testing.T) {
	raw := readFixture(t, "backdrop_sample.jpg")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/jpeg")
		_, _ = w.Write(raw)
	}))
	defer srv.Close()

	blob := &capturingBlob{}
	svc := &BackdropService{blob: blob}

	url, err := svc.saveBackdrop(context.Background(), 12345, srv.URL)
	if err != nil {
		t.Fatalf("saveBackdrop: %v", err)
	}
	if !strings.HasSuffix(blob.key, ".webp") {
		t.Fatalf("落盘键应以 .webp 结尾(当前 %q, url=%q) —— 说明 WebP 分支被跳过, 多半是 backdrop_service.go 缺 `_ \"image/jpeg\"`",
			blob.key, url)
	}
	if len(blob.data) < 12 || !bytes.HasPrefix(blob.data, []byte("RIFF")) || string(blob.data[8:12]) != "WEBP" {
		t.Fatalf("落盘字节不是 WebP(缺 RIFF....WEBP 头), len=%d", len(blob.data))
	}
}

// TestImageDecodeRecognizesJPEGAndPNG 守卫 service 包注册了 jpeg/png 解码器
// (backdrop_service.go 的两条空白 import)。缺任一条这里即失败。
func TestImageDecodeRecognizesJPEGAndPNG(t *testing.T) {
	cases := map[string]string{
		"backdrop_sample.jpg": "jpeg",
		"decoder_sample.png":  "png",
	}
	for file, want := range cases {
		_, format, err := image.Decode(bytes.NewReader(readFixture(t, file)))
		if err != nil || format != want {
			t.Fatalf("%s: image.Decode format=%q err=%v, 期望 %q —— 检查 backdrop_service.go 是否遗漏对应格式的空白 import",
				file, format, err, want)
		}
	}
}
