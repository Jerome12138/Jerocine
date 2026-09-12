package service

import (
	"context"
	"testing"
	"time"

	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/platform/auth"
)

// ---- fake: 只覆盖用户管理用到的仓储方法 ----

// userPtr 构造带 ID 的用户指针(gorm.Model 内嵌, 测试里只关心主键)。
func userPtr(id uint) *entity.User {
	u := entity.User{}
	u.ID = id
	return &u
}

type manageFakeUsers struct {
	repository.UserRepository
	u           *entity.User // GetById 命中目标
	listRows    []entity.User
	gotKeyword  string
	setDisabled bool
	setID       uint
	updatedPw   string
}

func (f *manageFakeUsers) GetByName(_ context.Context, name string) (*entity.User, error) {
	if f.u == nil || f.u.UserName != name {
		return nil, domain.ErrUserNotFound
	}
	return f.u, nil
}

func (f *manageFakeUsers) GetById(_ context.Context, id uint) (*entity.User, error) {
	if f.u != nil && f.u.ID == id {
		return f.u, nil
	}
	return nil, domain.ErrUserNotFound
}

func (f *manageFakeUsers) ListPaged(_ context.Context, keyword string, _ repository.Page) ([]entity.User, int64, error) {
	f.gotKeyword = keyword
	return f.listRows, int64(len(f.listRows)), nil
}

func (f *manageFakeUsers) SetDisabled(_ context.Context, id uint, disabled bool) error {
	f.setID, f.setDisabled = id, disabled
	return nil
}

func (f *manageFakeUsers) UpdatePassword(_ context.Context, _ uint, hashed string) error {
	f.updatedPw = hashed
	return nil
}

func newManageTestService(t *testing.T, u repository.UserRepository) *UserService {
	t.Helper()
	return NewUserService(u, nil, nil, nil, nil, testTokenManager(t), 2, time.Hour)
}

func TestManageSetUserDisabled_SelfForbidden(t *testing.T) {
	fu := &manageFakeUsers{u: userPtr(7)}
	s := newManageTestService(t, fu)
	if err := s.ManageSetUserDisabled(context.Background(), 7, 7, true); err != domain.ErrInvalidArgument {
		t.Fatalf("不能禁用自己: want ErrInvalidArgument, got %v", err)
	}
}

func TestManageSetUserDisabled_NotFound(t *testing.T) {
	fu := &manageFakeUsers{u: userPtr(7)}
	s := newManageTestService(t, fu)
	if err := s.ManageSetUserDisabled(context.Background(), 7, 99, true); err != domain.ErrUserNotFound {
		t.Fatalf("目标不存在: want ErrUserNotFound, got %v", err)
	}
}

func TestManageSetUserDisabled_DisableAndEnable(t *testing.T) {
	fu := &manageFakeUsers{u: userPtr(9)}
	s := newManageTestService(t, fu)
	if err := s.ManageSetUserDisabled(context.Background(), 7, 9, true); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if fu.setID != 9 || !fu.setDisabled {
		t.Fatalf("仓储未收到禁用指令: id=%d disabled=%v", fu.setID, fu.setDisabled)
	}
	if err := s.ManageSetUserDisabled(context.Background(), 7, 9, false); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if fu.setDisabled {
		t.Fatal("仓储应收到启用指令")
	}
}

func TestManageResetUserPassword(t *testing.T) {
	fu := &manageFakeUsers{u: userPtr(9)}
	s := newManageTestService(t, fu)
	// 过短拒绝
	if err := s.ManageResetUserPassword(context.Background(), 9, "123"); err != domain.ErrInvalidArgument {
		t.Fatalf("过短密码: want ErrInvalidArgument, got %v", err)
	}
	if err := s.ManageResetUserPassword(context.Background(), 9, "newpass123"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if fu.updatedPw == "" || fu.updatedPw == "newpass123" {
		t.Fatal("密码必须哈希后落库")
	}
}

func TestManageListUsers_PassesKeyword(t *testing.T) {
	fu := &manageFakeUsers{listRows: []entity.User{{UserName: "alice"}}}
	s := newManageTestService(t, fu)
	rows, total, err := s.ManageListUsers(context.Background(), "ali", repository.Page{Current: 1})
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatalf("list: err=%v total=%d rows=%d", err, total, len(rows))
	}
	if fu.gotKeyword != "ali" {
		t.Fatalf("keyword 未透传: %q", fu.gotKeyword)
	}
}

func TestLogin_RejectsDisabledUser(t *testing.T) {
	hashed, err := auth.HashPassword("secret123")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	bob := userPtr(9)
	bob.UserName = "bob"
	bob.Password = hashed
	bob.Disabled = 1
	fu := &manageFakeUsers{u: bob}
	s := newManageTestService(t, fu)
	// 禁用用户: 密码正确也统一 ErrUnauthorized(不泄漏账号状态)
	if _, err := s.Login(context.Background(), "bob", "secret123"); err != domain.ErrUnauthorized {
		t.Fatalf("禁用用户登录: want ErrUnauthorized, got %v", err)
	}
}
