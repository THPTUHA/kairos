package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"github.com/rs/zerolog/log"
	uuid "github.com/satori/go.uuid"
)

type Auth struct {
	HmacSecret string
	HmrfSecret string
}

var cf *Auth

func Init(hmacSecret string, hmrfSecret string) {
	cf = &Auth{
		HmacSecret: hmacSecret,
		HmrfSecret: hmrfSecret,
	}
}

type AccessDetails struct {
	UserID   string
	ClientID string
	UserType int
}

type TokenDetails struct {
	AccessToken string
	TokenUuid   string
}

type TokenManager struct{}

func NewTokenService() *TokenManager {
	return &TokenManager{}
}

type TokenInterface interface {
	CreateToken(userId string, userType int) (*TokenDetails, error)
	ExtractAccess(*http.Request) (*AccessDetails, error)
}

func (t *TokenManager) CreateToken(userId, clientID, userType string) (*TokenDetails, error) {
	td := &TokenDetails{}
	td.TokenUuid = uuid.NewV4().String()

	var err error
	atClaims := jwt.MapClaims{}
	atClaims["user_id"] = userId
	atClaims["client_id"] = clientID
	atClaims["user_type"] = userType
	at := jwt.NewWithClaims(jwt.SigningMethodHS256, atClaims)
	td.AccessToken, err = at.SignedString([]byte(cf.HmacSecret))
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}
	return td, nil
}

func extractToken(r *http.Request) string {
	bearToken := r.Header.Get("Authorization")
	strArr := strings.Split(bearToken, " ")
	if len(strArr) == 2 {
		return strArr[1]
	}
	return ""
}

func VerifyToken(r *http.Request) (*jwt.Token, error) {
	tokenString := extractToken(r)
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(cf.HmacSecret), nil
	})
	if err != nil {
		log.Error().Stack().Err(err).Msg("verify token")
		return nil, err
	}
	return token, nil
}

func VerifyStrToken(tokenString string) (*jwt.Token, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(cf.HmacSecret), nil
	})
	if err != nil {
		log.Error().Stack().Err(err).Msg("verify token")
		return nil, err
	}
	return token, nil
}

func Extract(token *jwt.Token) (*AccessDetails, error) {
	claims, ok := token.Claims.(jwt.MapClaims)

	if ok && token.Valid {
		userId, userOk := claims["user_id"].(string)
		userType, userTypeOk := claims["user_type"].(string)
		clientID, clientOk := claims["client_id"].(string)
		if !ok || !userOk || !userTypeOk || !clientOk {
			return nil, errors.New("unauthorized")
		} else {
			t, _ := strconv.Atoi(userType)
			return &AccessDetails{
				UserID:   userId,
				UserType: t,
				ClientID: clientID,
			}, nil
		}
	}
	return nil, errors.New("something went wrong")
}

func (t *TokenManager) ExtractAccess(r *http.Request) (*AccessDetails, error) {
	token, err := VerifyToken(r)
	if err != nil {
		return nil, err
	}
	acc, err := Extract(token)
	if err != nil {
		return nil, err
	}
	return acc, nil
}

func ExtractAccess(r *http.Request) (*AccessDetails, error) {
	token, err := VerifyToken(r)
	if err != nil {
		return nil, err
	}
	acc, err := Extract(token)
	if err != nil {
		return nil, err
	}
	return acc, nil
}

func ExtractAccessStr(t string) (*AccessDetails, error) {
	token, err := VerifyStrToken(t)
	if err != nil {
		return nil, err
	}
	acc, err := Extract(token)
	if err != nil {
		return nil, err
	}
	return acc, nil
}
