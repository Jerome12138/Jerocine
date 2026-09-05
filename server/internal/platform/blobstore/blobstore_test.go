package blobstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePath_RejectsTraversal(t *testing.T) {
	s := &localStore{dir: t.TempDir(), baseURL: "/api/upload/"}
	bad := []string{"../evil.txt", "a/../../evil", "/etc/passwd", "..", "", "foo;rm -rf", "x\x00y", "中文.jpg"}
	for _, k := range bad {
		if _, err := s.resolvePath(k); err == nil {
			t.Fatalf("expected reject for %q", k)
		}
	}
	good := []string{"poster/1.jpg", "a/b/c.png", "v_1.2.3.apk"}
	for _, k := range good {
		if _, err := s.resolvePath(k); err != nil {
			t.Fatalf("expected accept for %q, got %v", k, err)
		}
	}
}

func TestSaveDeleteRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := NewLocal(dir, "/api/upload/")
	ctx := context.Background()
	url, err := s.Save(ctx, "poster/1.jpg", []byte("img"))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if url != "/api/upload/poster/1.jpg" {
		t.Fatalf("url: %s", url)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "poster", "1.jpg")); err != nil || string(b) != "img" {
		t.Fatalf("file not written: %v", err)
	}
	if err := s.Delete(ctx, "poster/1.jpg"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	// 穿越 key 在 Save 即被拒
	if _, err := s.Save(ctx, "../escape.txt", []byte("x")); err == nil {
		t.Fatal("traversal save should fail")
	}
}

func TestSaveFromURL_RejectsPrivate(t *testing.T) {
	s := NewLocal(t.TempDir(), "/api/upload/")
	// 内网/环回目标必须被 SSRF 防护拒绝
	for _, u := range []string{"http://127.0.0.1/x.jpg", "http://localhost/x.jpg", "ftp://example.com/x", "http://169.254.169.254/latest"} {
		if _, err := s.SaveFromURL(context.Background(), "p/x.jpg", u); err == nil {
			t.Fatalf("expected SSRF reject for %q", u)
		}
	}
}
