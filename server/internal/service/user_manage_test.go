package service

import (
	"context"
	"strings"
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
	other       *entity.User // GetByName 的第二个账号(测重名)
	listRows    []entity.User
	gotKeyword  string
	setDisabled bool
	setID       uint
	updatedPw   string
	updatedID   uint
	updatedName string
	updatedRole int
	deletedID   uint
}

func (f *manageFakeUsers) GetByName(_ context.Context, name string) (*entity.User, error) {
	for _, u := range []*entity.User{f.u, f.other} {
		if u != nil && u.UserName == name {
			return u, nil
		}
	}
	return nil, domain.ErrUserNotFound
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

func (f *manageFakeUsers) UpdateProfile(_ context.Context, id uint, name string, role int) error {
	f.updatedID, f.updatedName, f.updatedRole = id, name, role
	return nil
}

func (f *manageFakeUsers) Delete(_ context.Context, id uint) error {
	if f.u == nil || f.u.ID != id {
		return domain.ErrUserNotFound
	}
	f.deletedID = id
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

func TestManageUpdateUser_RenameAndRole(t *testing.T) {
	bob := userPtr(9)
	bob.UserName = "bob"
	fu := &manageFakeUsers{u: bob}
	s := newManageTestService(t, fu)
	if err := s.ManageUpdateUser(context.Background(), 7, 9, "bobby", 1); err != nil {
		t.Fatalf("update: %v", err)
	}
	if fu.updatedID != 9 || fu.updatedName != "bobby" || fu.updatedRole != 1 {
		t.Fatalf("仓储未收到编辑指令: id=%d name=%q role=%d", fu.updatedID, fu.updatedName, fu.updatedRole)
	}
}

func TestManageUpdateUser_InvalidArgs(t *testing.T) {
	fu := &manageFakeUsers{u: userPtr(9)}
	s := newManageTestService(t, fu)
	cases := []struct {
		name, userName string
		role           int
	}{
		{"空用户名", "  ", 0},
		{"超长用户名", strings.Repeat("x", 33), 0},
		{"非法角色", "bob", 2},
	}
	for _, tc := range cases {
		if err := s.ManageUpdateUser(context.Background(), 7, 9, tc.userName, tc.role); err != domain.ErrInvalidArgument {
			t.Fatalf("%s: want ErrInvalidArgument, got %v", tc.name, err)
		}
	}
}

func TestManageUpdateUser_SelfRoleChangeForbidden(t *testing.T) {
	bob := userPtr(7)
	bob.UserName = "bob"
	bob.Role = entity.RoleUser
	fu := &manageFakeUsers{u: bob}
	s := newManageTestService(t, fu)
	if err := s.ManageUpdateUser(context.Background(), 7, 7, "bob", entity.RoleAdmin); err != domain.ErrInvalidArgument {
		t.Fatalf("改自己角色: want ErrInvalidArgument, got %v", err)
	}
	// 自己改名(角色不变)是允许的
	if err := s.ManageUpdateUser(context.Background(), 7, 7, "bobby", entity.RoleUser); err != nil {
		t.Fatalf("自己改名: %v", err)
	}
}

func TestManageUpdateUser_DuplicateNameConflict(t *testing.T) {
	bob := userPtr(9)
	bob.UserName = "bob"
	alice := userPtr(10)
	alice.UserName = "alice"
	fu := &manageFakeUsers{u: bob, other: alice}
	s := newManageTestService(t, fu)
	if err := s.ManageUpdateUser(context.Background(), 7, 9, "alice", 0); err != domain.ErrConflict {
		t.Fatalf("重名: want ErrConflict, got %v", err)
	}
	// 改回自己的名字不冲突
	if err := s.ManageUpdateUser(context.Background(), 7, 9, "bob", 0); err != nil {
		t.Fatalf("名字不变: %v", err)
	}
}

func TestManageDeleteUser(t *testing.T) {
	bob := userPtr(9)
	bob.UserName = "bob"
	fu := &manageFakeUsers{u: bob}
	s := newManageTestService(t, fu)
	// 不能删自己
	if err := s.ManageDeleteUser(context.Background(), 7, 7); err != domain.ErrInvalidArgument {
		t.Fatalf("删自己: want ErrInvalidArgument, got %v", err)
	}
	// 目标不存在
	if err := s.ManageDeleteUser(context.Background(), 7, 99); err != domain.ErrUserNotFound {
		t.Fatalf("删不存在: want ErrUserNotFound, got %v", err)
	}
	if err := s.ManageDeleteUser(context.Background(), 7, 9); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if fu.deletedID != 9 {
		t.Fatalf("仓储未收到删除指令: id=%d", fu.deletedID)
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
