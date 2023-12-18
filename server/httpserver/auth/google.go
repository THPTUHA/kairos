package auth

import (
	"context"
	"encoding/gob"
	"fmt"
	"net/http"

	"github.com/THPTUHA/kairos/server/httpserver/config"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/golang/glog"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	goauth "google.golang.org/api/oauth2/v2"
	"google.golang.org/api/option"
)

type Credentials struct {
	ClientID     string `json:"clientid"`
	ClientSecret string `json:"secret"`
}

var (
	conf  *oauth2.Config
	store sessions.Store
)

const (
	StateKey  = "state"
	sessionID = "ginoauth_google_session"
)

func init() {
	gob.Register(goauth.Userinfo{})
}

func Session(name string) gin.HandlerFunc {
	return sessions.Sessions(name, store)
}

func Setup(redirectURL string, scopes []string, secret []byte, cfg *config.Configs) {
	store = cookie.NewStore(secret)

	conf = &oauth2.Config{
		ClientID:     cfg.Auth.ClientID,
		ClientSecret: cfg.Auth.ClientSecret,
		RedirectURL:  redirectURL,
		Scopes:       scopes,
		Endpoint:     google.Endpoint,
	}
}

func GetLoginURL(state string) string {
	return conf.AuthCodeURL(state)
}

func GoogleAuth() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		tok, err := conf.Exchange(context.TODO(), ctx.Query("code"))
		if err != nil {
			fmt.Printf("failed to exchange code for oauth token: %v", err)
			ctx.AbortWithError(http.StatusBadRequest, fmt.Errorf("failed to exchange code for oauth token: %w", err))
			return
		}
		oAuth2Service, err := goauth.NewService(ctx, option.WithTokenSource(conf.TokenSource(ctx, tok)))
		if err != nil {
			glog.Errorf("[Gin-OAuth] Failed to create oauth service: %v", err)
			ctx.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to create oauth service: %w", err))
			return
		}

		userInfo, err := oAuth2Service.Userinfo.Get().Do()
		if err != nil {
			glog.Errorf("[Gin-OAuth] Failed to get userinfo for user: %v", err)
			ctx.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to get userinfo for user: %w", err))
			return
		}
		ctx.Set("user", userInfo.Email)
		ctx.Set("avatar", userInfo.Picture)
	}
}
