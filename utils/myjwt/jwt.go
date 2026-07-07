package myjwt

import (
	"my-AIchat/config"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type MyClaim struct {
	Username string `json:"username"`
	ID       int64  `json:"id"`
	jwt.RegisteredClaims
}

func GenerateToken(username string, id int64) (string, error) {
	claim := MyClaim{
		Username: username,
		ID:       id,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(config.GetConfig().JwtConfig.ExpireDuration) * time.Hour)),
			Issuer:    config.GetConfig().JwtConfig.Issuer,
			Subject:   config.GetConfig().JwtConfig.Subject,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	//生成token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claim)
	return token.SignedString([]byte(config.GetConfig().JwtConfig.Key))
}

func ParseToken(token string) (MyClaim, bool) {
	claim := MyClaim{}
	t, err := jwt.ParseWithClaims(token, &claim, func(t *jwt.Token) (interface{}, error) {
		return []byte(config.GetConfig().JwtConfig.Key), nil
	})
	if err != nil || !t.Valid {
		return MyClaim{}, false
	}
	return claim, true
}
