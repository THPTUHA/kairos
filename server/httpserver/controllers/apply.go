package controllers

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

func ApplyYaml(c *gin.Context) {
	userID := c.GetString("user_id")
	fmt.Printf("userID = %s", userID)

	c.JSON(http.StatusOK, gin.H{
		"data":    gin.H{},
		"message": "apply success",
	})
}
