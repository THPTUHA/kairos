package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/THPTUHA/kairos/pkg/helper"
	"github.com/THPTUHA/kairos/pkg/orderedmap"
	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/THPTUHA/kairos/server/storage"
	"github.com/THPTUHA/kairos/server/storage/models"
	"github.com/dop251/goja"
	"github.com/gin-gonic/gin"
)

func validFunction(jsCode string) error {
	re := regexp.MustCompile(`function\s+(\w+)\s*\([^)]*\)\s*{[^{}]*}`)
	match := re.FindStringSubmatch(jsCode)
	if len(match) > 2 {
		return fmt.Errorf("more than one function")
	}
	if len(match) > 1 {
		functionIndex := re.FindStringIndex(jsCode)
		beforeFunction := jsCode[:functionIndex[0]]
		afterFunction := jsCode[functionIndex[1]:]
		newJsCode := beforeFunction + afterFunction
		if strings.TrimSpace(newJsCode) != "" {
			return fmt.Errorf("only one function")
		}
	}

	return nil
}

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
	if err := validFunction(function.Content); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"err": err.Error(),
		})
		return
	}
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

func (ctr *Controller) DeleteFunction(c *gin.Context) {
	fid := c.Query("fid")
	id, _ := strconv.ParseInt(fid, 10, 64)
	err := storage.DeleteFunction(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "delete function",
	})
}

type Debug struct {
	Input map[string]workflow.ReplyData `json:"input"`
	Flows string                        `json:"flows"`
}

func (ctr *Controller) Debug(c *gin.Context) {
	userID, _ := c.Get("userID")
	var debug Debug
	err := c.BindJSON(&debug)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"err": err.Error(),
		})
		return
	}

	if debug.Flows == "" {
		c.JSON(http.StatusOK, gin.H{
			"error":    "empty code!",
			"output":   "",
			"tracking": "",
		})
		return
	}

	t := workflow.NewTemplate(nil)
	vars := &workflow.Vars{
		OrderedMap: orderedmap.New[string, *workflow.Var](),
	}

	input := make([]string, 0)
	for k := range debug.Input {
		input = append(input, k)
	}

	err = t.Build(input, debug.Flows, vars, false)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"error":    err.Error(),
			"output":   "",
			"tracking": "",
		})
		return
	}

	if len(t.FuncNotFound) != 0 {
		uid, _ := strconv.ParseInt(userID.(string), 10, 64)
		funcs, err := storage.GetAllFunctionsByUserID(uid)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"err": err.Error(),
			})
			return
		}
		for _, ff := range t.FuncNotFound {
			exist := false
			for _, f := range funcs {
				if f.Name == ff {
					exist = true
					break
				}
			}
			if !exist {
				c.JSON(http.StatusOK, gin.H{
					"error":    fmt.Sprintf("'%s' is not defined", ff),
					"output":   "",
					"tracking": "",
				})
				return
			}
		}

		vm := goja.New()

		fc := &workflow.FuncCall{
			Call:  make(map[string]goja.Callable),
			Funcs: vm,
		}
		for _, f := range funcs {
			prog, err := goja.Compile("", f.Content, true)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"err": err.Error(),
				})
				return
			}
			_, err = fc.Funcs.RunProgram(prog)
			if err != nil {
				c.JSON(http.StatusOK, gin.H{
					"error":    err.Error(),
					"output":   "",
					"tracking": "",
				})
				return
			}
			v := fc.Funcs.Get(f.Name)
			if v == nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"err": fmt.Sprintf("%s not found", f.Name),
				})
				return
			}

			var fr goja.Callable
			err = fc.Funcs.ExportTo(v, &fr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{
					"err": fmt.Sprintf("%s not found", f.Name),
				})
			}
			fc.Call[f.Name] = fr
		}

		t.SetFuncs(fc)
	}

	fmt.Printf("DEBUG %+v \n", debug)

	tr := t.NewRutime(debug.Input)
	exeOuput := tr.Execute()
	tracking, _ := json.Marshal(exeOuput.Tracking)

	c.JSON(http.StatusOK, gin.H{
		"message":  "debug function",
		"output":   exeOuput.DeliverFlows,
		"tracking": string(tracking),
		"error":    exeOuput.Tracking.Err,
	})
}
