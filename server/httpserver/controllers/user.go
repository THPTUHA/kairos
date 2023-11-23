package controllers

import (
	"database/sql"
	"net/http"

	"github.com/THPTUHA/kairos/server/storage"
	"github.com/gin-gonic/gin"
)

func (ctr *Controller) InfoUser(c *gin.Context) {
	userID, _ := c.Get("userID")
	user, err := storage.GetInfoUser(userID.(string))
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusUnauthorized, "")
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Info user",
		"user":    &user,
	})
}
