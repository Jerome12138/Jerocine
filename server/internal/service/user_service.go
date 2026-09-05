package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"server/internal/cache"
	"server/internal/domain"
	"server/internal/domain/entity"
	"server/internal/domain/repository"
	"server/internal/platform/auth"
)

// UserService 认证 + 观看历史 + 收藏编排。
type UserService struct {
	users      repository.UserRepository
	history    repository.HistoryRepository
	favorite   repository.FavoriteRepository
	skips      repository.SkipSettingRepository
	search     repository.SearchRepository // 历史/收藏卡片回填
	tokens     *auth.TokenManager
	maxDevices int
	ttl        time.Duration
}

func NewUserService(u repository.UserRepository, h repository.HistoryRepository, f repository.FavoriteRepository, sk repository.SkipSettingRepository, s repository.SearchRepository, tm *auth.TokenManager, maxDevices int, ttl time.Duration) *UserService {
	return &UserService{users: u, history: h, favorite: f, skips: sk, search: s, tokens: tm, maxDevices: maxDevices, ttl: ttl}
}

// ---- 片头片尾跳过设置(账号级, 跨设备) ----

// SkipList 取用户全部跳过设置。
func (s *UserService) SkipList(ctx context.Context, userId int64) ([]entity.UserSkipSetting, error) {
	if s.skips == nil {
		return []entity.UserSkipSetting{}, nil
	}
	return s.skips.List(ctx, userId)
}

// SkipSave upsert 某片跳过设置, 秒数夹到 [0, 600]。
func (s *UserService) SkipSave(ctx context.Context, userId, mid int64, intro, outro int) error {
	if s.skips == nil {
		return nil
	}
	clamp := func(v int) int {
		if v < 0 {
			return 0
		}
		if v > 600 {
			return 600
		}
		return v
	}
	return s.skips.Upsert(ctx, &entity.UserSkipSetting{
		UserId: userId, Mid: mid, IntroSec: clamp(intro), OutroSec: clamp(outro),
	})
}

// SkipReset 删除某片跳过设置(回归默认)。
func (s *UserService) SkipReset(ctx context.Context, userId, mid int64) error {
	if s.skips == nil {
		return nil
	}
	return s.skips.Delete(ctx, userId, mid)
}

// LoginResult 登录响应(对齐 OpenAPI LoginResp)。
type LoginResult struct {
	UserName string `json:"userName"`
	Token    string `json:"token"`
	Expires  int64  `json:"expires"`
	Role     int    `json:"role"`
}

// HistoryView 历史 + 影片卡片回填。
type HistoryView struct {
	entity.UserHistory
	Card *entity.MovieSearch `json:"card"`
}

// FavoriteView 收藏 + 影片卡片回填。
type FavoriteView struct {
	entity.UserFavorite
	Card *entity.MovieSearch `json:"card"`
}

// Login 校验账号密码, 签发 token 并登记到多设备集合。失败统一 ErrUnauthorized(不泄漏原因)。
func (s *UserService) Login(ctx context.Context, account, password string) (LoginResult, error) {
	u, err := s.users.GetByName(ctx, account)
	if err == domain.ErrUserNotFound {
		return LoginResult{}, domain.ErrUnauthorized
	}
	if err != nil {
		return LoginResult{}, err
	}
	if !auth.VerifyPassword(password, u.Password) {
		return LoginResult{}, domain.ErrUnauthorized
	}
	token, exp, err := s.tokens.Generate(u.ID, u.UserName, u.Role)
	if err != nil {
		return LoginResult{}, err
	}
	if err := cache.SaveUserToken(ctx, int64(u.ID), token, s.maxDevices, s.ttl); err != nil {
		return LoginResult{}, err
	}
	return LoginResult{UserName: u.UserName, Token: token, Expires: exp, Role: u.Role}, nil
}

// Authenticate 中间件用: 解析 token + 校验是否在该用户有效集合内。
func (s *UserService) Authenticate(ctx context.Context, token string) (*auth.Claims, error) {
	claims, err := s.tokens.Parse(token)
	if err != nil {
		return nil, domain.ErrUnauthorized
	}
	if !cache.IsUserTokenValid(ctx, int64(claims.UserID), token) {
		return nil, domain.ErrUnauthorized
	}
	return claims, nil
}

func (s *UserService) Logout(ctx context.Context, userID uint, token string) {
	cache.ClearOneToken(ctx, int64(userID), token)
}

// ---- 扫码登录(设备码流程) ----

const deviceCodeTTL = 5 * time.Minute

// DeviceCode 生成一对 deviceCode(轮询用)/userCode(二维码给手机扫), 存 Redis 待确认。
func (s *UserService) DeviceCode(ctx context.Context) (deviceCode, userCode string, expiresIn int, err error) {
	deviceCode, err = randHex(24)
	if err != nil {
		return "", "", 0, err
	}
	userCode, err = randUserCode(8)
	if err != nil {
		return "", "", 0, err
	}
	if err = cache.SaveDeviceCode(ctx, deviceCode, userCode, deviceCodeTTL); err != nil {
		return "", "", 0, err
	}
	return deviceCode, userCode, int(deviceCodeTTL.Seconds()), nil
}

// DeviceConfirm 手机端(已登录)确认授权某 userCode。码不存在/过期返回 ErrInvalidArgument。
func (s *UserService) DeviceConfirm(ctx context.Context, userCode string, userID uint) error {
	ok, err := cache.AuthorizeByUserCode(ctx, strings.ToUpper(strings.TrimSpace(userCode)), int64(userID), deviceCodeTTL)
	if err != nil {
		return err
	}
	if !ok {
		return domain.ErrInvalidArgument
	}
	return nil
}

// DevicePoll 待登录端轮询: pending/expired 时 token 为空; authorized 时签发 token 并一次性删码。
// status: "pending" | "expired" | "ok"。
func (s *UserService) DevicePoll(ctx context.Context, deviceCode string) (LoginResult, string, error) {
	da, err := cache.GetDeviceCode(ctx, deviceCode)
	if err != nil || da == nil {
		return LoginResult{}, "expired", nil
	}
	if da.Status != cache.DeviceCodeAuthorized {
		return LoginResult{}, "pending", nil
	}
	u, err := s.users.GetById(ctx, uint(da.UserID))
	if err != nil {
		return LoginResult{}, "", err
	}
	token, exp, err := s.tokens.Generate(u.ID, u.UserName, u.Role)
	if err != nil {
		return LoginResult{}, "", err
	}
	if err := cache.SaveUserToken(ctx, int64(u.ID), token, s.maxDevices, s.ttl); err != nil {
		return LoginResult{}, "", err
	}
	cache.DeleteDeviceCode(ctx, deviceCode, da.UserCode) // 一次性
	return LoginResult{UserName: u.UserName, Token: token, Expires: exp, Role: u.Role}, "ok", nil
}

// randHex n 字节加密随机 → 十六进制串。
func randHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// randUserCode n 位易读短码(去掉 B/I/O/0/1/8 等易混字符)。
func randUserCode(n int) (string, error) {
	const alphabet = "ACDEFGHJKLMNPQRSTUVWXYZ2345679"
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return string(b), nil
}

func (s *UserService) Me(ctx context.Context, userID uint) (*entity.User, error) {
	return s.users.GetById(ctx, userID)
}

func (s *UserService) ChangePassword(ctx context.Context, userID uint, oldPw, newPw string) error {
	u, err := s.users.GetById(ctx, userID)
	if err != nil {
		return err
	}
	if !auth.VerifyPassword(oldPw, u.Password) {
		return domain.ErrUnauthorized
	}
	hashed, err := auth.HashPassword(newPw)
	if err != nil {
		return domain.ErrInvalidArgument
	}
	if err := s.users.UpdatePassword(ctx, userID, hashed); err != nil {
		return err
	}
	cache.ClearUserTokens(ctx, int64(userID)) // 改密踢下线全部设备
	return nil
}

// ---- 管理员: 创建/列出用户 ----

func (s *UserService) CreateUser(ctx context.Context, name, password string, role int) (*entity.User, error) {
	if _, err := s.users.GetByName(ctx, name); err == nil {
		return nil, domain.ErrConflict
	} else if err != domain.ErrUserNotFound {
		return nil, err
	}
	hashed, err := auth.HashPassword(password)
	if err != nil {
		return nil, domain.ErrInvalidArgument
	}
	u := &entity.User{UserName: name, Password: hashed, Role: role}
	if err := s.users.Create(ctx, u); err != nil {
		return nil, err
	}
	return u, nil
}

func (s *UserService) ListUsers(ctx context.Context, page repository.Page) ([]entity.User, int64, error) {
	return s.users.List(ctx, page.Normalize(20))
}

// ---- 观看历史 ----

func (s *UserService) HistoryUpsert(ctx context.Context, h *entity.UserHistory) error {
	return s.history.Upsert(ctx, h)
}

func (s *UserService) HistoryList(ctx context.Context, userID int64, page repository.Page) ([]HistoryView, int64, error) {
	page = page.Normalize(20)
	rows, total, err := s.history.List(ctx, userID, page)
	if err != nil {
		return nil, 0, err
	}
	mids := make([]int64, 0, len(rows))
	for _, r := range rows {
		mids = append(mids, r.Mid)
	}
	cards := s.cardMap(ctx, mids)
	out := make([]HistoryView, 0, len(rows))
	for _, r := range rows {
		v := HistoryView{UserHistory: r}
		if c, ok := cards[r.Mid]; ok {
			v.Card = c
		}
		out = append(out, v)
	}
	return out, total, nil
}

func (s *UserService) HistoryDelete(ctx context.Context, userID, mid int64) error {
	return s.history.Delete(ctx, userID, mid)
}

func (s *UserService) HistoryClear(ctx context.Context, userID int64) error {
	return s.history.Clear(ctx, userID)
}

// ---- 收藏 ----

func (s *UserService) FavoriteAdd(ctx context.Context, userID, mid int64) error {
	return s.favorite.Add(ctx, &entity.UserFavorite{UserId: userID, Mid: mid})
}

func (s *UserService) FavoriteRemove(ctx context.Context, userID, mid int64) error {
	return s.favorite.Remove(ctx, userID, mid)
}

func (s *UserService) FavoriteCheck(ctx context.Context, userID, mid int64) (bool, error) {
	return s.favorite.Exists(ctx, userID, mid)
}

func (s *UserService) FavoriteList(ctx context.Context, userID int64, page repository.Page) ([]FavoriteView, int64, error) {
	page = page.Normalize(20)
	rows, total, err := s.favorite.List(ctx, userID, page)
	if err != nil {
		return nil, 0, err
	}
	mids := make([]int64, 0, len(rows))
	for _, r := range rows {
		mids = append(mids, r.Mid)
	}
	cards := s.cardMap(ctx, mids)
	out := make([]FavoriteView, 0, len(rows))
	for _, r := range rows {
		v := FavoriteView{UserFavorite: r}
		if c, ok := cards[r.Mid]; ok {
			v.Card = c
		}
		out = append(out, v)
	}
	return out, total, nil
}

// cardMap 批量取卡片用于历史/收藏回填。
func (s *UserService) cardMap(ctx context.Context, mids []int64) map[int64]*entity.MovieSearch {
	m := make(map[int64]*entity.MovieSearch)
	if len(mids) == 0 {
		return m
	}
	cards, err := s.search.GetByMids(ctx, mids)
	if err != nil {
		return m
	}
	for i := range cards {
		m[cards[i].Mid] = &cards[i]
	}
	return m
}
