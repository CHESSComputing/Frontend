package main

// server module
//
// Copyright (c) 2023 - Valentin Kuznetsov <vkuznet@gmail.com>
//
import (
	"embed"
	"fmt"
	"log"

	beamlines "github.com/CHESSComputing/golib/beamlines"
	srvConfig "github.com/CHESSComputing/golib/config"
	server "github.com/CHESSComputing/golib/server"
	services "github.com/CHESSComputing/golib/services"
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

// global variables
var _beamlines []string
var _smgr beamlines.SchemaManager
var _oauthServer *oauthServer.Server
var _httpReadRequest, _httpWriteRequest *services.HttpRequest
var _header, _footer string
var Verbose int

// helper function to define our header
func header() string {
	if _header == "" {
		tmpl := server.MakeTmpl(StaticFs, "Header")
		tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
		_header = server.TmplPage(StaticFs, "header.tmpl", tmpl)
	}
	return _header
}

// helper function to define our footer
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

// helper function to initialize our router
func setupRouter() *gin.Engine {
	routes := []server.Route{
		server.Route{Method: "GET", Path: "/", Handler: MainHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/docs", Handler: DocsHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/docs/:page", Handler: DocsHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/login", Handler: LoginHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/logout", Handler: LogoutHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/services", Handler: ServicesHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/search", Handler: SearchHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/meta", Handler: MetaDataHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/provenance", Handler: ProvenanceHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/aiml", Handler: AIMLHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/analysis", Handler: AnalysisHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/visualization", Handler: VisualizationHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/data", Handler: DataHandler, Authorized: false},
		server.Route{Method: "POST", Path: "/login", Handler: KAuthHandler, Authorized: false},
		server.Route{Method: "POST", Path: "/search", Handler: SearchHandler, Authorized: false},
		server.Route{Method: "POST", Path: "/dbs/files", Handler: DBSFilesHandler, Authorized: false},
		server.Route{Method: "POST", Path: "/meta/form/upload", Handler: MetaFormUploadHandler, Authorized: false},
		server.Route{Method: "POST", Path: "/meta/file/upload", Handler: MetaFileUploadHandler, Authorized: false},
		server.Route{Method: "POST", Path: "/populateform", Handler: UploadJsonHandler, Authorized: false},
	}
	r := server.Router(routes, StaticFs, "static", srvConfig.Config.Frontend.WebServer)
	r.Use(server.CounterMiddleware())
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

	// initialize schema manager
	_smgr = beamlines.SchemaManager{}
	for _, fname := range srvConfig.Config.CHESSMetaData.SchemaFiles {
		_, err := _smgr.Load(fname)
		if err != nil {
			log.Fatalf("unable to load %s error %v", fname, err)
		}
		_beamlines = append(_beamlines, utils.FileName(fname))
	}
	log.Println("Schema", _smgr.String())

	// initialize http request
	_httpReadRequest = services.NewHttpRequest("read", Verbose)
	_httpWriteRequest = services.NewHttpRequest("write", Verbose)

	// initialize router
	r := setupRouter()
	sport := fmt.Sprintf(":%d", srvConfig.Config.Frontend.WebServer.Port)
	log.Printf("Start HTTP server %s", sport)
	r.Run(sport)
}
