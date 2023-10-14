package controllers

import (
	"fmt"
	"net/http"

	"github.com/THPTUHA/kairos/server/httpserver/auth"
	"github.com/THPTUHA/kairos/server/storage/models"
	"github.com/THPTUHA/kairos/server/storage/repos"
	"github.com/gin-gonic/gin"
)

var tokenManager = auth.TokenManager{}

func Login(c *gin.Context) {
	var u models.User
	if err := c.ShouldBindJSON(&u); err != nil {
		c.JSON(http.StatusUnprocessableEntity, "Invalid json provided")
		return
	}

	fmt.Print(u)
	user, _ := repos.UserRepo.FindByUserName(u.Username)

	ts, err := tokenManager.CreateToken(fmt.Sprint(user.ID), user.Username)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, err.Error())
		return
	}

	token := map[string]string{
		"access_token": ts.AccessToken,
	}

	c.JSON(http.StatusOK, token)
}
