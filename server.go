package main

// server module
//
// Copyright (c) 2023 - Valentin Kuznetsov <vkuznet@gmail.com>
//
// The OAuth parts are based on
// https://github.com/dghubble/gologin
// package where we explid github authentication, see
// https://github.com/dghubble/gologin/blob/main/examples/github

import (
	"embed"
	"log"

	beamlines "github.com/CHESSComputing/golib/beamlines"
	srvConfig "github.com/CHESSComputing/golib/config"
	server "github.com/CHESSComputing/golib/server"
	services "github.com/CHESSComputing/golib/services"
	utils "github.com/CHESSComputing/golib/utils"
	"github.com/gin-gonic/gin"
)

// content is our static web server content.
//
//go:embed static
var StaticFs embed.FS

// global variables
var _beamlines []string
var _smgr beamlines.SchemaManager
var _httpReadRequest, _httpWriteRequest *services.HttpRequest
var _header, _footer, _footerEmpty string
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

// helper function to define our footer
func footerEmpty() string {
	if _footerEmpty == "" {
		tmpl := server.MakeTmpl(StaticFs, "Footer")
		tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
		_footerEmpty = server.TmplPage(StaticFs, "footer_empty.tmpl", tmpl)
	}
	return _footerEmpty
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
		server.Route{Method: "GET", Path: "/tools", Handler: ToolsHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/meta", Handler: MetaDataHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/specscans", Handler: SpecScansHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/notebook", Handler: NotebookHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/publish", Handler: PublishSrvHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/aiml", Handler: AIMLHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/analysis", Handler: AnalysisHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/visualization", Handler: VisualizationHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/data", Handler: DataHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/datasets", Handler: DatasetsHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/dstable", Handler: DatasetsTableHandler, Authorized: false},
		server.Route{Method: "GET", Path: "/provinfo", Handler: ProvInfoHandler, Authorized: false},
		server.Route{Method: "POST", Path: "/login", Handler: KAuthHandler, Authorized: false},
		server.Route{Method: "POST", Path: "/search", Handler: SearchHandler, Authorized: false},
		server.Route{Method: "POST", Path: "/provenance", Handler: ProvenanceHandler, Authorized: false},
		server.Route{Method: "POST", Path: "/meta/form/upload", Handler: MetaFormUploadHandler, Authorized: false},
		server.Route{Method: "POST", Path: "/meta/file/upload", Handler: MetaFileUploadHandler, Authorized: false},
		server.Route{Method: "POST", Path: "/publish", Handler: PublishHandler, Authorized: false},
		server.Route{Method: "POST", Path: "/publishform", Handler: PublishFormHandler, Authorized: false},
		server.Route{Method: "POST", Path: "/populateform", Handler: UploadJsonHandler, Authorized: false},
	}
	r := server.Router(routes, StaticFs, "static", srvConfig.Config.Frontend.WebServer)

	// OAuth routes
	for _, arec := range srvConfig.Config.Frontend.OAuth {
		if arec.Provider == "github" {
			r.GET("/github/login", GithubOauthLoginHandler)
			r.GET("/github/callback", GithubCallBackHandler)
			log.Println("github oauth is enabled")
		} else if arec.Provider == "google" {
			r.GET("/google/login", GoogleOauthLoginHandler)
			r.GET("/google/callback", GoogleCallBackHandler)
			log.Println("google oauth is enabled")
		} else if arec.Provider == "facebook" {
			r.GET("/facebook/login", FacebookOauthLoginHandler)
			r.GET("/facebook/callback", FacebookCallBackHandler)
			log.Println("facebook oauth is enabled")
		}
	}
	return r
}

// Server defines our HTTP server
func Server() {
	// set Verbose level
	Verbose = srvConfig.Config.Frontend.Verbose

	// set default end-point
	if srvConfig.Config.Frontend.DefaultEndPoint != "" {
		DEFAULT_END_POINT = srvConfig.Config.Frontend.DefaultEndPoint
	} else {
		DEFAULT_END_POINT = "/dstable"
	}

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

	// setup web router and start the service
	r := setupRouter()
	webServer := srvConfig.Config.Frontend.WebServer
	server.StartServer(r, webServer)
}
