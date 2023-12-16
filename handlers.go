package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"time"

	authz "github.com/CHESSComputing/golib/authz"
	srvConfig "github.com/CHESSComputing/golib/config"
	server "github.com/CHESSComputing/golib/server"
	"github.com/gin-gonic/gin"
	"github.com/go-oauth2/oauth2/v4"
	"gopkg.in/jcmturner/gokrb5.v7/credentials"
)

// Documentation about gib handlers can be found over here:
// https://go.dev/doc/tutorial/web-service-gin

//
// Data structure we use through the code
//

// DocsParams represents URI storage params in /docs/:page end-point
type DocsParams struct {
	Page string `uri:"page" binding:"required"`
}

// User represents structure used by users DB in Authz service to handle incoming requests
type User struct {
	Login    string
	Password string
}

//
// helper functions
//

// helper function to provide error page
func handleError(c *gin.Context, msg string, err error) {
	page := server.ErrorPage(StaticFs, msg, err)
	c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+page+footer()))
}

// helper function to provides error template message
func errorTmpl(c *gin.Context, msg string, err error) string {
	tmpl := server.MakeTmpl(StaticFs, "Status")
	tmpl["Content"] = template.HTML(fmt.Sprintf("<div>%s</div>\n<br/><h3>ERROR</h3>%v", msg, err))
	content := server.TmplPage(StaticFs, "error.tmpl", tmpl)
	return content
}

// helper functiont to provides success template message
func successTmpl(c *gin.Context, msg string) string {
	tmpl := server.MakeTmpl(StaticFs, "Status")
	tmpl["Content"] = template.HTML(fmt.Sprintf("<h3>SUCCESS</h3><div>%s</div>", msg))
	content := server.TmplPage(StaticFs, "success.tmpl", tmpl)
	return content
}

//
// GET handlers
//

// helper function to validate user and generate token
func validateUser(c *gin.Context) (oauth2.GrantType, *oauth2.TokenGenerateRequest, error) {
	var gt oauth2.GrantType
	gt = "client_credentials"
	tgr := &oauth2.TokenGenerateRequest{
		ClientID:     srvConfig.Config.Authz.ClientID,
		ClientSecret: srvConfig.Config.Authz.ClientSecret,
		Request:      c.Request,
	}
	return gt, tgr, nil
}

// KAuthHandler provides kerberos authentication handler
func KAuthHandler(c *gin.Context) {
	// get http request/writer
	//     w := c.Writer
	r := c.Request

	user, err := c.Cookie("user")
	if err == nil && user != "" {
		log.Println("found user cookie", user)
		c.Redirect(http.StatusFound, "/search")
		return
	}

	expiration := time.Now().Add(24 * time.Hour)
	// in test mode we'll set user as TestUser
	if srvConfig.Config.Frontend.TestMode {
		log.Println("frontend test mode")
		c.Set("user", "TestUser")
		//         cookie := http.Cookie{Name: "user", Value: "TestUser", Expires: expiration}
		//         http.SetCookie(w, &cookie)
		c.Redirect(http.StatusFound, "/search")
		return
	}

	/*
		// if in test mode or do not use keytab
		if srvConfig.Config.Kerberos.Keytab == "" || srvConfig.Config.Frontend.TestMode {
			gt, treq, err := validateUser(c)
			if err != nil {
				msg := "wrong user credentials"
				handleError(c, msg, err)
				return
			}
			tokenInfo, err := _oauthServer.GetAccessToken(c, gt, treq)
			if err != nil {
				msg := "wrong access token"
				handleError(c, msg, err)
				return
			}
			// set custom token attributes
			duration := srvConfig.Config.Authz.TokenExpires
			if duration > 0 {
				tokenInfo.SetCodeExpiresIn(time.Duration(duration))
			}
			tmap := _oauthServer.GetTokenData(tokenInfo)
			data, err := json.MarshalIndent(tmap, "", "  ")
			if err != nil {
				msg := "fail to marshal token map"
				handleError(c, msg, err)
				return
			}

			tmpl := server.MakeTmpl(StaticFs, "Success")
			tmpl["Content"] = fmt.Sprintf("<br/>Generated token:<br/><pre>%s</pre>", string(data))
			page := server.TmplPage(StaticFs, "success.tmpl", tmpl)
			w.Write([]byte(header() + page + footer()))
			return
		}
	*/

	// First, we need to get the value of the `code` query param
	err = r.ParseForm()
	if err != nil {
		content := server.ErrorPage(StaticFs, "could not parse http form", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}
	name := r.FormValue("name")
	password := r.FormValue("password")
	var creds *credentials.Credentials
	if name != "" && password != "" {
		creds, err = kuser(name, password)
		if err != nil {
			content := server.ErrorPage(StaticFs, "wrong user credentials", err)
			c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
			return
		}
	} else {
		content := server.ErrorPage(StaticFs, "user/password is empty", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}
	if creds == nil {
		content := server.ErrorPage(StaticFs, "unable to obtain user credentials", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}

	// store user name in c.Context
	c.Set("user", name)
	//     c.SetCookie("user", name, expiration, "/", domain string, secure, httpOnly bool) {
	cookie := http.Cookie{Name: "user", Value: name, Expires: expiration}
	http.SetCookie(c.Writer, &cookie)
	log.Println("KAuthHandler set cookie user", name)
	c.Redirect(http.StatusFound, "/search")
}

// MainHandler provides access to GET / end-point
func MainHandler(c *gin.Context) {
	// check if user cookie is set, this is necessary as we do not
	// use authorization handler for / end-point
	user, err := c.Cookie("user")
	if err == nil {
		c.Set("user", user)
	} else {
		LoginHandler(c)
	}
}

// LoginHandler provides access to GET /login endpoint
func LoginHandler(c *gin.Context) {
	tmpl := server.MakeTmpl(StaticFs, "Login")
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	content := server.TmplPage(StaticFs, "login.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// LogoutHandler provides access to GET /logout endpoint
func LogoutHandler(c *gin.Context) {
	c.SetCookie("user", "", -1, "/", domain(), false, true)
	c.Redirect(http.StatusFound, "/")
}

// ServicesHandler provides access to GET / end-point
func ServicesHandler(c *gin.Context) {
	user, err := c.Cookie("user")
	if err != nil {
		LoginHandler(c)
	}
	if srvConfig.Config.Frontend.WebServer.Verbose > 0 {
		log.Printf("user from c.Cookie: '%s'", user)
	}

	// top and bottom HTTP content from our templates
	tmpl := server.MakeTmpl(StaticFs, "Home")
	tmpl["LogoClass"] = "show"
	tmpl["MapClass"] = "hide"
	if user != "" {
		tmpl["LogoClass"] = "hide"
		tmpl["MapClass"] = "show"
		tmpl["Users"] = user
	}
	content := server.TmplPage(StaticFs, "index.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// DocsHandler provides access to GET /docs end-point
func DocsHandler(c *gin.Context) {
	// check if user cookie is set, this is necessary as we do not
	// use authorization handler for /docs end-point
	tmpl := server.MakeTmpl(StaticFs, "Documentation")
	tmpl["Title"] = "Documentation"
	fname := "static/markdown/main.md"
	var params DocsParams
	if err := c.ShouldBindUri(&params); err == nil {
		fname = fmt.Sprintf("static/markdown/%s", params.Page)
	}
	content, err := mdToHTML(fname)
	if err != nil {
		content = fmt.Sprintf("unable to convert %s to HTML, error %v", fname, err)
		log.Println("ERROR: ", content)
		tmpl["Content"] = content
	}
	tmpl["Content"] = template.HTML(content)
	content = server.TmplPage(StaticFs, "content.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// SearchHandler provides access to GET /search endpoint
func SearchHandler(c *gin.Context) {
	w := c.Writer
	r := c.Request
	user, err := c.Cookie("user")
	log.Println("SearchHandler", user, err)
	if err != nil {
		LoginHandler(c)
		return
	}

	// create search template form
	tmpl := server.MakeTmpl(StaticFs, "Search")

	// if we got GET request it is /search web form
	if r.Method == "GET" {
		tmpl["Query"] = ""
		tmpl["User"] = user
		tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
		page := server.TmplPage(StaticFs, "searchform.tmpl", tmpl)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(header() + page + footer()))
		return
	}

	// if we get POST request we'll process user query
	query := r.FormValue("query")
	if Verbose > 0 {
		log.Printf("search query='%s' user=%v", query, user)
	}
	if err != nil {
		msg := "unable to parse user query"
		handleError(c, msg, err)
		return
	}

	// TODO: send query to Discovery Service
	page := "TODO: send query to Discovery Service"
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+page+footer()))
}

// MetaDataHandler provides access to GET /search endpoint
func MetaDataHandler(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+"Not Implemented"+footer()))
}

// ProvenanceHandler provides access to GET /search endpoint
func ProvenanceHandler(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+"Not Implemented"+footer()))
}

// POST handlers

// LoginPostHandler provides access to POST /login endpoint
func LoginPostHandler(c *gin.Context) {
	var form authz.LoginForm
	var content string
	var err error

	if err = c.ShouldBind(&form); err != nil {
		content = errorTmpl(c, "login form binding error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}

	// encrypt provided user password before sending to Authz server
	form, err = authz.EncryptLoginObject(form)
	if err != nil {
		content = errorTmpl(c, "unable to encrypt user password", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}

	// make a call to Authz service to check for a user
	rurl := fmt.Sprintf("%s/oauth/authorize?client_id=%s&response_type=code", srvConfig.Config.Services.AuthzURL, srvConfig.Config.Authz.ClientID)
	user := User{Login: form.User, Password: form.Password}
	data, err := json.Marshal(user)
	if err != nil {
		content = errorTmpl(c, "unable to marshal user form, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}
	resp, err := http.Post(rurl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		content = errorTmpl(c, "unable to POST request to Authz service, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}
	defer resp.Body.Close()
	data, err = io.ReadAll(resp.Body)
	var response authz.Response
	err = json.Unmarshal(data, &response)
	if err != nil {
		content = errorTmpl(c, "unable handle authz response, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}
	if srvConfig.Config.Frontend.WebServer.Verbose > 0 {
		log.Printf("INFO: Authz response %+v, error %v", response, err)
	}
	if response.Status != "ok" {
		msg := fmt.Sprintf("No user %s found in Authz service", form.User)
		content = errorTmpl(c, msg, errors.New("user not found"))
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}

	c.Set("user", form.User)
	if srvConfig.Config.Frontend.WebServer.Verbose > 0 {
		log.Printf("login from user %s, url path %s", form.User, c.Request.URL.Path)
	}

	// set our user cookie
	if _, err := c.Cookie("user"); err != nil {
		if srvConfig.Config.Frontend.WebServer.Verbose > 0 {
			log.Printf("set cookie user=%s domain=%s", form.User, domain())
		}
		c.SetCookie("user", form.User, 3600, "/", domain(), false, true)
	}

	// redirect
	c.Redirect(http.StatusFound, "/")
}
