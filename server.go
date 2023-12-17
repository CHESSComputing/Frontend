package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"

	srvConfig "github.com/CHESSComputing/golib/config"
	"github.com/CHESSComputing/golib/server"
	utils "github.com/CHESSComputing/golib/utils"
	"github.com/gin-gonic/gin"
	"github.com/go-oauth2/oauth2/v4/generates"
	"github.com/go-oauth2/oauth2/v4/manage"
	"github.com/go-oauth2/oauth2/v4/models"
	oauthServer "github.com/go-oauth2/oauth2/v4/server"
	"github.com/go-oauth2/oauth2/v4/store"
	"github.com/golang-jwt/jwt"
)

// content is our static web server content.
//
//go:embed static
var StaticFs embed.FS

var _oauthServer *oauthServer.Server
var _header, _footer string
var Verbose int

func header() string {
	if _header == "" {
		tmpl := server.MakeTmpl(StaticFs, "Header")
		tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
		_header = server.TmplPage(StaticFs, "header.tmpl", tmpl)
	}
	return _header
}
func footer() string {
	if _footer == "" {
		tmpl := server.MakeTmpl(StaticFs, "Footer")
		tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
		_footer = server.TmplPage(StaticFs, "footer.tmpl", tmpl)
	}
	return _footer
}

// helper function to handle base path of URL requests
func base(api string) string {
	b := srvConfig.Config.Frontend.WebServer.Base
	return utils.BasePath(b, api)
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
	r.GET("/services", ServicesHandler)
	r.GET("/search", SearchHandler)
	r.GET("/meta", MetaDataHandler)
	r.GET("/provenance", ProvenanceHandler)
	r.GET("/aiml", AIMLHandler)
	r.GET("/analysis", AnalysisHandler)
	r.GET("/visualization", VisualizationHandler)
	r.GET("/data", DataHandler)

	// POST end-poinst
	//     r.POST("/login", LoginPostHandler)
	r.POST("/login", KAuthHandler)

	// static files
	for _, dir := range []string{"js", "css", "images", "templates"} {
		filesFS, err := fs.Sub(StaticFs, "static/"+dir)
		if err != nil {
			panic(err)
		}
		m := fmt.Sprintf("%s/%s", srvConfig.Config.Frontend.WebServer.Base, dir)
		r.StaticFS(m, http.FS(filesFS))
	}

	r.GET("/", MainHandler)
	return r
}

// Server defines our HTTP server
func Server() {
	// set Verbose level
	Verbose = srvConfig.Config.Frontend.Verbose

	// setup oauth parts
	manager := manage.NewDefaultManager()
	manager.SetAuthorizeCodeTokenCfg(manage.DefaultAuthorizeCodeTokenCfg)

	// token store
	manager.MustTokenStorage(store.NewMemoryTokenStore())

	// generate jwt access token
	manager.MapAccessGenerate(
		generates.NewJWTAccessGenerate(
			"", []byte(srvConfig.Config.Authz.ClientID), jwt.SigningMethodHS512))
	//     manager.MapAccessGenerate(generates.NewAccessGenerate())

	clientStore := store.NewClientStore()
	clientStore.Set(srvConfig.Config.Authz.ClientID, &models.Client{
		ID:     srvConfig.Config.Authz.ClientID,
		Secret: srvConfig.Config.Authz.ClientSecret,
		Domain: srvConfig.Config.Authz.Domain,
	})
	manager.MapClientStorage(clientStore)
	_oauthServer = oauthServer.NewServer(oauthServer.NewConfig(), manager)
	_oauthServer.SetAllowGetAccessRequest(true)
	_oauthServer.SetClientInfoHandler(oauthServer.ClientFormHandler)
	r := setupRouter()
	sport := fmt.Sprintf(":%d", srvConfig.Config.Frontend.WebServer.Port)
	log.Printf("Start HTTP server %s", sport)
	r.Run(sport)
}
