package controllers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/THPTUHA/kairos/pkg/helper"
	"github.com/THPTUHA/kairos/server/storage"
	"github.com/THPTUHA/kairos/server/storage/models"
	"github.com/dop251/goja"
	"github.com/gin-gonic/gin"
)

func (ctr *Controller) CreateFunction(c *gin.Context) {
	userID, _ := c.Get("userID")
	var function models.Function
	err := c.BindJSON(&function)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}
	uid, _ := strconv.ParseInt(userID.(string), 10, 64)
	vm := goja.New()
	prog, err := goja.Compile("", function.Content, true)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"err": err.Error(),
		})
		return
	}
	_, err = vm.RunProgram(prog)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"err": err.Error(),
		})
		return
	}
	_, ok := goja.AssertFunction(vm.Get(function.Name))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": fmt.Errorf("can't find function"),
		})
		return
	}

	function.CreatedAt = helper.GetTimeNow()
	function.UserID = uid
	fmt.Printf("FUNCTION %+v\n", function)
	id, err := storage.InsertFunction(&function)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "create functions success",
		"functions": id,
	})
}

func (ctr *Controller) ListFunction(c *gin.Context) {
	userID, _ := c.Get("userID")
	uid, _ := strconv.ParseInt(userID.(string), 10, 64)
	functions, err := storage.GetAllFunctionsByUserID(uid)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":   "list functions",
		"functions": functions,
	})
}
