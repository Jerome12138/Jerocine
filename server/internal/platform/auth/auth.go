// Package auth 负责 JWT 签发/校验与密码哈希。从旧 model/system/Jwt.go + util.StringUtil 平移收敛。
package auth

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// Claims JWT 载荷, 把角色编进去便于中间件直接判定。
type Claims struct {
	UserID   uint   `json:"userID"`
	UserName string `json:"userName"`
	Role     int    `json:"role"`
	jwt.RegisteredClaims
}

// TokenManager RS256 签发/校验。
type TokenManager struct {
	priv   *rsa.PrivateKey
	pub    *rsa.PublicKey
	ttl    time.Duration
	issuer string
}

// NewTokenManager 解析 PKCS1 PEM 公私钥。
func NewTokenManager(privPEM, pubPEM []byte, ttl time.Duration, issuer string) (*TokenManager, error) {
	priv, err := parsePriv(privPEM)
	if err != nil {
		return nil, err
	}
	pub, err := parsePub(pubPEM)
	if err != nil {
		return nil, err
	}
	if ttl <= 0 {
		ttl = 168 * time.Hour
	}
	return &TokenManager{priv: priv, pub: pub, ttl: ttl, issuer: issuer}, nil
}

// Generate 签发 token, 返回 token 串与过期时间戳(秒)。
func (t *TokenManager) Generate(userID uint, userName string, role int) (string, int64, error) {
	exp := time.Now().Add(t.ttl)
	c := Claims{
		UserID: userID, UserName: userName, Role: role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    t.issuer,
			Subject:   userName,
			Audience:  jwt.ClaimStrings{"Auth_All"},
			ExpiresAt: jwt.NewNumericDate(exp),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-10 * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	s, err := jwt.NewWithClaims(jwt.SigningMethodRS256, c).SignedString(t.priv)
	if err != nil {
		return "", 0, err
	}
	return s, exp.Unix(), nil
}

// Parse 校验并解析 token。过期会返回 (claims, err) 以便上层区分。
func (t *TokenManager) Parse(tokenStr string) (*Claims, error) {
	// 安全: 显式锁定 RS256, 防 alg 混淆攻击(攻击者用 alg=HS256 拿公钥当 HMAC 密钥伪造 token)。
	token, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(tok *jwt.Token) (interface{}, error) {
		if _, ok := tok.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errors.New("auth: unexpected signing method")
		}
		return t.pub, nil
	}, jwt.WithValidMethods([]string{"RS256"}))
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			claims, _ := token.Claims.(*Claims)
			return claims, err
		}
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("token invalid")
	}
	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, errors.New("invalid claims type")
	}
	return claims, nil
}

func parsePriv(buf []byte) (*rsa.PrivateKey, error) {
	p, _ := pem.Decode(buf)
	if p == nil {
		return nil, errors.New("auth: private key PEM decode failed")
	}
	return x509.ParsePKCS1PrivateKey(p.Bytes)
}

func parsePub(buf []byte) (*rsa.PublicKey, error) {
	p, _ := pem.Decode(buf)
	if p == nil {
		return nil, errors.New("auth: public key PEM decode failed")
	}
	return x509.ParsePKCS1PublicKey(p.Bytes)
}

// ---- 密码哈希 ----

const bcryptCost = 10

// HashPassword bcrypt 哈希(salt 自管)。
func HashPassword(pw string) (string, error) {
	if pw == "" {
		return "", errors.New("password is empty")
	}
	if len(pw) > 72 {
		return "", errors.New("密码长度不能超过 72 字节")
	}
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcryptCost)
	return string(b), err
}

// VerifyPassword 校验 bcrypt 密码。
func VerifyPassword(pw, hash string) bool {
	if hash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// IsBcrypt 判断是否 bcrypt 哈希(用于老 md5 哈希透明迁移)。
func IsBcrypt(s string) bool {
	return strings.HasPrefix(s, "$2a$") || strings.HasPrefix(s, "$2b$") || strings.HasPrefix(s, "$2y$")
}
