package controllers

import (
	"crypto/rand"
	"encoding/base64"
	"log"

	"github.com/THPTUHA/kairos/server/httpserver/auth"
	"github.com/THPTUHA/kairos/server/httpserver/helper"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func randToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("[Gin-OAuth] Failed to read rand: %v", err)
	}
	return base64.StdEncoding.EncodeToString(b)
}

func Login(c *gin.Context) {
	stateValue := randToken()
	session := sessions.Default(c)
	session.Set(auth.StateKey, stateValue)
	session.Save()
	c.Writer.Write([]byte(helper.AutoRedirctUrl(auth.GetLoginURL(stateValue))))
}
