package auth

import (
	"fmt"
	"time"

	"github.com/THPTUHA/kairos/server/serverhttp/config"
	"github.com/golang-jwt/jwt/v4"
	"github.com/rs/zerolog/log"
)

var cf *config.Configs

type CustomClaims struct {
	UserID int `json:"user_id"`
	jwt.RegisteredClaims
}

func Init(config *config.Configs) {
	cf = config
}

func Generate(userID int) string {
	claims := CustomClaims{
		userID,
		jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(30 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "user",
			Subject:   "auth",
			ID:        fmt.Sprint(userID),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	ss, err := token.SignedString([]byte(cf.Auth.HmacSecret))

	if err != nil {
		log.Error().Msg(err.Error())
		return ""
	}
	return ss
}

func Validate(tokenString string) int {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(cf.Auth.HmacSecret), nil
	})

	if err != nil {
		log.Error().Msg(err.Error())
		return 0
	}

	if claims, ok := token.Claims.(*CustomClaims); ok && token.Valid {
		return claims.UserID
	} else {
		log.Error().Msg(err.Error())
		return 0
	}
}
