package controllers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func ApplyYaml(c *gin.Context) {
	// var collection workflow.Collection
	// err := c.BindJSON(&collection)
	// // get user...
	// collection.Username = "abc"

	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{
	// 		"error": err.Error(),
	// 	})
	// 	return
	// }

	// collection.Path = fmt.Sprintf("%s/%s", collection.Username, collection.Namespace)
	// // pending workflow, task khi khởi tạo collection

	// err = storage.Store.SetCollection(&collection)
	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{
	// 		"error": err.Error(),
	// 	})
	// 	return
	// }
	// storage.Store.GetWorkflows(&agent.WorkflowOptions{})
	// // check remote server exist collection??

	// // TODO check liệu kv storage có lưu trong disk ko
	// cs, err := storage.Store.GetCollections(&agent.CollectionOptions{Namespace: collection.Namespace, Username: collection.Username})
	// if len(cs) > 0 {
	// 	fmt.Printf("%d tasks found \n", len(cs))
	// }

	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{
	// 		"error": err.Error(),
	// 	})
	// 	return
	// }
	c.JSON(http.StatusOK, gin.H{
		"data":    gin.H{},
		"message": "apply success",
	})
}
