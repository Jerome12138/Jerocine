package service

import (
	"context"
	"testing"

	"server/internal/domain/entity"
)

// suppressedHealthRepo 始终返回 Suppressed=true 的健康快照(模拟该源已被自动停采)。
type suppressedHealthRepo struct{}

func (suppressedHealthRepo) Get(context.Context, string) (*entity.SourceHealth, error) {
	return &entity.SourceHealth{Suppressed: true, Status: entity.HealthDown}, nil
}
func (suppressedHealthRepo) List(context.Context) ([]entity.SourceHealth, error) { return nil, nil }
func (suppressedHealthRepo) Upsert(context.Context, *entity.SourceHealth) error  { return nil }

// ClientOnly 源即便健康表里有 Suppressed=true 旧行, 也永不被判为停采(防御性兜底)。
func TestIsSuppressed_ClientOnlyNeverSuppressed(t *testing.T) {
	s := &SpiderService{health: suppressedHealthRepo{}}
	clientOnly := &entity.CollectSource{Id: "bf", ClientOnly: true}
	if s.isSuppressed(context.Background(), clientOnly) {
		t.Fatal("ClientOnly 源不应被自动停采")
	}
	// 同样的停采健康行, 普通源应判为已停采(对照组)。
	normal := &entity.CollectSource{Id: "norm"}
	if !s.isSuppressed(context.Background(), normal) {
		t.Fatal("普通源命中 Suppressed 行应判为已停采")
	}
}

// nil 源安全返回 false(不 panic)。
func TestIsSuppressed_NilSource(t *testing.T) {
	s := &SpiderService{health: suppressedHealthRepo{}}
	if s.isSuppressed(context.Background(), nil) {
		t.Fatal("nil 源应返回 false")
	}
}
