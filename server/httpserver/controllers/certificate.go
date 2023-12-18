package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/THPTUHA/kairos/server/storage"
	"github.com/THPTUHA/kairos/server/storage/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type req struct {
	Name               string                      `json:"name"`
	ChannelPermissions []*models.ChannelPermission `json:"channel_permissions"`
}

func (ctr *Controller) CreateCertificate(c *gin.Context) {
	userID, _ := c.Get("userID")
	uid, _ := strconv.ParseInt(userID.(string), 10, 64)
	var req req

	err := c.BindJSON(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	apiKey := uuid.New().String()
	expireAt := time.Now().Unix() + 100000
	cert, err := storage.CreateCertificate(
		req.Name,
		uid,
		apiKey,
		expireAt,
		req.ChannelPermissions,
		func(certID int64) (string, error) {
			r, err := ctr.TokenService.CreateToken(userID.(string), fmt.Sprint(certID), fmt.Sprint(models.ChannelUser))
			fmt.Println("Create err", err)
			if err != nil {
				return "", err
			}
			return r.AccessToken, nil
		},
	)

	fmt.Println("Cert err", err)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "create certificate success",
		"certificate": cert,
	})
}

func (ctr *Controller) ListCertificate(c *gin.Context) {
	userID, _ := c.Get("userID")
	uid, _ := strconv.ParseInt(userID.(string), 10, 64)
	certs, err := storage.GetCertificatesByUserID(uid)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "list certificate",
		"certificates": certs,
	})
}

func (ctr *Controller) PermisCert(c *gin.Context) {
	userID, _ := c.Get("userID")
	clientID, exist := c.Get("clientID")
	apiKey := c.GetHeader("api-key")

	if !exist || userID == "" || apiKey == "" {
		c.JSON(http.StatusUnauthorized, nil)
		return
	}
	cid, err := strconv.ParseInt(clientID.(string), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}
	cert, err := storage.GetCertificatesID(cid)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	if cert.APIKey != apiKey {
		c.JSON(http.StatusUnauthorized, nil)
		return
	}

	cp, err := storage.GetChannelInfoByCertID(cid)
	if err != nil {
		ctr.Log.Error(err)
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":     "permission",
		"permissions": cp,
	})
}
