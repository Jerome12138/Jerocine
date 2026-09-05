package service

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/platform/auth"
)

func testTokenManager(t *testing.T) *auth.TokenManager {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PUBLIC KEY", Bytes: x509.MarshalPKCS1PublicKey(&key.PublicKey)})
	tm, err := auth.NewTokenManager(privPEM, pubPEM, time.Hour, "test")
	if err != nil {
		t.Fatalf("token manager: %v", err)
	}
	return tm
}

type fakeUsers struct {
	repository.UserRepository
	u *entity.User
}

func (f *fakeUsers) GetByName(_ context.Context, name string) (*entity.User, error) {
	if f.u == nil || f.u.UserName != name {
		return nil, domain.ErrUserNotFound
	}
	return f.u, nil
}

type fakeHistory struct {
	repository.HistoryRepository
	rows []entity.UserHistory
}

func (f *fakeHistory) List(context.Context, int64, repository.Page) ([]entity.UserHistory, int64, error) {
	return f.rows, int64(len(f.rows)), nil
}

type fakeUserSearch struct {
	repository.SearchRepository
	cards []entity.MovieSearch
}

func (f *fakeUserSearch) GetByMids(context.Context, []int64) ([]entity.MovieSearch, error) {
	return f.cards, nil
}

func TestLoginAndAuthenticate(t *testing.T) {
	setupCache(t)
	tm := testTokenManager(t)
	hash, _ := auth.HashPassword("secret")
	u := &entity.User{UserName: "admin", Password: hash, Role: 1}
	u.ID = 10000
	svc := NewUserService(&fakeUsers{u: u}, nil, nil, nil, nil, tm, 5, time.Hour)

	res, err := svc.Login(context.Background(), "admin", "secret")
	if err != nil {
		t.Fatalf("login err: %v", err)
	}
	if res.Token == "" || res.UserName != "admin" || res.Role != 1 {
		t.Fatalf("login result: %+v", res)
	}
	claims, err := svc.Authenticate(context.Background(), res.Token)
	if err != nil || claims.UserID != 10000 {
		t.Fatalf("authenticate: claims=%+v err=%v", claims, err)
	}

	if _, err := svc.Login(context.Background(), "admin", "wrong"); err != domain.ErrUnauthorized {
		t.Fatalf("wrong pw should be ErrUnauthorized, got %v", err)
	}
	if _, err := svc.Login(context.Background(), "ghost", "secret"); err != domain.ErrUnauthorized {
		t.Fatalf("unknown user should be ErrUnauthorized, got %v", err)
	}
}

func TestHistoryListEnrichesCards(t *testing.T) {
	setupCache(t)
	tm := testTokenManager(t)
	hist := &fakeHistory{rows: []entity.UserHistory{{Mid: 1, Episode: 3}, {Mid: 2}}}
	fsearch := &fakeUserSearch{cards: []entity.MovieSearch{{Mid: 1, Name: "甲", Cover: "c1"}}}
	svc := NewUserService(&fakeUsers{}, hist, nil, nil, fsearch, tm, 5, time.Hour)

	views, total, err := svc.HistoryList(context.Background(), 10000, repository.Page{Current: 1, Size: 20})
	if err != nil || total != 2 || len(views) != 2 {
		t.Fatalf("history list: total=%d len=%d err=%v", total, len(views), err)
	}
	if views[0].Card == nil || views[0].Card.Name != "甲" {
		t.Fatalf("mid=1 should be enriched: %+v", views[0])
	}
	if views[1].Card != nil {
		t.Fatalf("mid=2 has no card, should be nil")
	}
}
