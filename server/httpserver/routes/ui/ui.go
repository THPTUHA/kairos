package ui

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"io/ioutil"
	"log"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

//go:embed dist
var uiDist embed.FS

const uiPathPrefix = "ui/"

func UI(r *gin.RouterGroup) *template.Template {
	r.GET("/", func(c *gin.Context) {
		switch c.NegotiateFormat(gin.MIMEHTML) {
		case gin.MIMEHTML:
			c.Redirect(http.StatusSeeOther, "/ui/")
		default:
			c.AbortWithStatus(http.StatusNotFound)
		}
	})

	// r.Group("/" + uiPathPrefix)
	ui := r.Group("/" + uiPathPrefix)
	assets, err := fs.Sub(uiDist, "dist")
	if err != nil {
		log.Fatalln(err)
	}
	a, err := assets.Open("index.html")
	if err != nil {
		log.Fatalln(err)
	}
	b, err := ioutil.ReadAll(a)
	if err != nil {
		log.Fatalln(err)
	}
	t, err := template.New("index.html").Parse(string(b))
	if err != nil {
		log.Fatalln(err)
	}

	ui.GET("/*filepath", func(ctx *gin.Context) {
		p := ctx.Param("filepath")
		fmt.Printf("File here %s\n", p)

		f := strings.TrimPrefix(p, "/")
		_, err := assets.Open(f)
		if err == nil && p != "/" && p != "/index.html" {
			ctx.FileFromFS(p, http.FS(assets))
		}

		ctx.HTML(http.StatusOK, "index.html", gin.H{})
	})

	return t
}
