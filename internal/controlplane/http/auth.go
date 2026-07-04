package http

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const BcryptCost = 10 // bcrypt.DefaultCost，统一项目级常量

// AuthClaims 统一 JWT claims（per D-01）
type AuthClaims struct {
	jwt.RegisteredClaims
	UserID string `json:"user_id"`
	Role   string `json:"role"` // "admin" | "user"
}

// GenerateAuthToken 签发带 user_id + role 的 JWT（per D-01）
func GenerateAuthToken(secret []byte, userID, role string, expiry time.Duration) (string, error) {
	now := time.Now()
	claims := AuthClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			Issuer:    "claudedock",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
		},
		UserID: userID,
		Role:   role,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(secret)
}
