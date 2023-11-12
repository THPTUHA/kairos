package auth

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

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
	TokenUuid  string
	UserID     string
	UserName   string
	ClientName string
	KairosName string
}

type TokenDetails struct {
	AccessToken string
	TokenUuid   string
	AtExpires   int64
	RtExpires   int64
}

type TokenManager struct{}

func NewTokenService() *TokenManager {
	return &TokenManager{}
}

type TokenInterface interface {
	CreateToken(userId, userName string) (*TokenDetails, error)
	ExtractAccess(*http.Request) (*AccessDetails, error)
}

func (t *TokenManager) CreateClientToken(userId, userName, clientName, kairosName string) (*TokenDetails, error) {
	td := &TokenDetails{}
	td.AtExpires = time.Now().Add(time.Hour * 24 * 30).Unix() //expires after 30 min
	td.TokenUuid = uuid.NewV4().String()

	td.RtExpires = time.Now().Add(time.Hour * 24 * 7).Unix()

	var err error
	atClaims := jwt.MapClaims{}
	atClaims["access_uuid"] = td.TokenUuid
	atClaims["user_id"] = userId
	atClaims["user_name"] = userName
	atClaims["exp"] = td.AtExpires
	atClaims["client_name"] = clientName
	atClaims["kairos_name"] = kairosName

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

func (t *TokenManager) CreateToken(userId, userName string) (*TokenDetails, error) {
	td := &TokenDetails{}
	td.AtExpires = time.Now().Add(time.Hour * 24 * 30).Unix() //expires after 30 min
	td.TokenUuid = uuid.NewV4().String()

	td.RtExpires = time.Now().Add(time.Hour * 24 * 7).Unix()

	var err error
	atClaims := jwt.MapClaims{}
	atClaims["access_uuid"] = td.TokenUuid
	atClaims["user_id"] = userId
	atClaims["user_name"] = userName
	atClaims["exp"] = td.AtExpires
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
		accessUuid, ok := claims["access_uuid"].(string)
		userId, userOk := claims["user_id"].(string)
		userName, userNameOk := claims["user_name"].(string)
		clientName, _ := claims["client_name"].(string)
		kairosName, _ := claims["kairos_name"].(string)
		if !ok || !userOk || !userNameOk {
			return nil, errors.New("unauthorized")
		} else {
			return &AccessDetails{
				TokenUuid:  accessUuid,
				UserID:     userId,
				UserName:   userName,
				ClientName: clientName,
				KairosName: kairosName,
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
