package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"time"

	srvConfig "github.com/CHESSComputing/common/config"
	"github.com/gin-gonic/gin"
)

// content is our static web server content.
//
//go:embed static
var StaticFs embed.FS

// helper function to parse given template and return HTML page
func tmplPage(tmpl string, tmplData TmplRecord) string {
	if tmplData == nil {
		tmplData = make(TmplRecord)
	}
	var templates Templates
	page := templates.Tmpl(tmpl, tmplData)
	return page
}

// helper function to make initial template struct
func makeTmpl(c *gin.Context, title string) TmplRecord {
	tmpl := make(TmplRecord)
	tmpl["Title"] = title
	tmpl["User"] = ""
	if user, ok := c.Get("user"); ok {
		tmpl["User"] = user
	}
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	tmpl["ServerInfo"] = srvConfig.Info()
	tmpl["Top"] = tmplPage("top.tmpl", tmpl)
	tmpl["Bottom"] = tmplPage("bottom.tmpl", tmpl)
	tmpl["StartTime"] = time.Now().Unix()
	return tmpl
}

// helper function which sets gin router and defines all our server end-points
func setupRouter() *gin.Engine {
	// Disable Console Color
	// gin.DisableConsoleColor()
	r := gin.Default()

	// middlewares: https://gin-gonic.com/docs/examples/using-middleware/
	// Recovery middleware recovers from any panics and writes a 500 if there was one.
	r.Use(gin.Recovery())

	// GET end-points
	r.GET("/docs", DocsHandler)
	r.GET("/docs/:page", DocsHandler)
	r.GET("/login", LoginHandler)
	r.GET("/logout", LogoutHandler)
	r.GET("/user/registration", UserRegistryHandler)

	// captcha access
	r.GET("/captcha/:file", CaptchaHandler())

	// POST end-poinst
	r.POST("/login", LoginPostHandler)
	r.POST("/user/registration", UserRegistryPostHandler)

	// static files
	for _, dir := range []string{"js", "css", "images"} {
		filesFS, err := fs.Sub(StaticFs, "static/"+dir)
		if err != nil {
			panic(err)
		}
		m := fmt.Sprintf("%s/%s", srvConfig.Config.Frontend.WebServer.Base, dir)
		r.StaticFS(m, http.FS(filesFS))
	}

	r.GET("/", IndexHandler)
	return r
}

// Server defines our HTTP server
func Server() {
	r := setupRouter()
	sport := fmt.Sprintf(":%d", srvConfig.Config.Frontend.WebServer.Port)
	log.Printf("Start HTTP server %s", sport)
	r.Run(sport)
}
