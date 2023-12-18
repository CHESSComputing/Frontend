package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"strings"
	"time"

	authz "github.com/CHESSComputing/golib/authz"
	srvConfig "github.com/CHESSComputing/golib/config"
	"github.com/CHESSComputing/golib/mongo"
	server "github.com/CHESSComputing/golib/server"
	utils "github.com/CHESSComputing/golib/utils"
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
func handleError(c *gin.Context, status int, msg string, err error) {
	log.Printf("ERROR: %s %s, %v", status, msg, err)
	page := server.ErrorPage(StaticFs, msg, err)
	c.Data(status, "text/html; charset=utf-8", []byte(header()+page+footer()))
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
	w := c.Writer
	r := c.Request

	user, err := c.Cookie("user")
	if err == nil && user != "" {
		log.Println("found user cookie", user)
		c.Redirect(http.StatusFound, "/services")
		return
	}

	expiration := time.Now().Add(24 * time.Hour)
	// in test mode we'll set user as TestUser
	if srvConfig.Config.Frontend.TestMode {
		log.Println("frontend test mode")
		c.Set("user", "TestUser")
		cookie := http.Cookie{Name: "user", Value: "TestUser", Expires: expiration}
		http.SetCookie(w, &cookie)
		c.Redirect(http.StatusFound, "/services")
		return
	}

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
	cookie := http.Cookie{Name: "user", Value: name, Expires: expiration}
	http.SetCookie(w, &cookie)
	log.Println("KAuthHandler set cookie user", name)
	c.Redirect(http.StatusFound, "/services")
}

// MainHandler provides access to GET / end-point
func MainHandler(c *gin.Context) {
	// check if user cookie is set, this is necessary as we do not
	// use authorization handler for / end-point
	user, err := c.Cookie("user")
	if err == nil {
		c.Set("user", user)
		ServicesHandler(c)
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
	if Verbose > 0 {
		log.Printf("user from c.Cookie: '%s'", user)
	}

	// top and bottom HTTP content from our templates
	tmpl := server.MakeTmpl(StaticFs, "Home")
	tmpl["MapClass"] = "hide"
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	content := server.TmplPage(StaticFs, "services.tmpl", tmpl)
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
	user, err := c.Cookie("user")
	log.Println("SearchHandler", user, err, c.Request.Method)
	if err != nil {
		LoginHandler(c)
		return
	}

	// create search template form
	tmpl := server.MakeTmpl(StaticFs, "Search")
	tmpl["Query"] = ""
	tmpl["User"] = user
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base

	// if we got GET request it is /search web form
	if c.Request.Method == "GET" {
		page := server.TmplPage(StaticFs, "searchform.tmpl", tmpl)
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+page+footer()))
		return
	}

	// if we get POST request we'll process user query
	query := c.Request.FormValue("query")
	if Verbose > 0 {
		log.Printf("search query='%s' user=%v", query, user)
	}
	if err != nil {
		msg := "unable to parse user query"
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}

	// obtain valid token
	scope := "read"
	_httpRequest.GetToken(scope)

	// create POST payload
	rec := make(map[string]string)
	rec["query"] = query
	rec["user"] = user
	rec["client"] = "frontend"
	data, err := json.Marshal(rec)
	if err != nil {
		msg := "unable to parse user query"
		handleError(c, http.StatusInternalServerError, msg, err)
		return
	}
	// create POST form payload
	//     contentType := "application/x-www-form-urlencoded"
	//     form := url.Values{}
	//     form.Add("user", user)
	//     form.Add("query", string(query))
	//     form.Add("client", "cli")

	// TODO: replace request to Discovery service
	// place request to MetaData service
	rurl := fmt.Sprintf("%s", srvConfig.Config.Services.MetaDataURL)
	//     resp, err := _httpRequest.Post(rurl, contentType, strings.NewReader(form.Encode()))
	resp, err := _httpRequest.Post(rurl, "application/json", bytes.NewBuffer(data))
	log.Println("### MetaData response %+v, error %v", resp, err)

	// TODO: send query to Discovery Service
	page := "TODO: send query to Discovery Service"
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+page+footer()))
}

// MetaDataHandler provides access to GET /meta endpoint
func MetaDataHandler(c *gin.Context) {
	user, err := c.Cookie("user")
	if err != nil {
		LoginHandler(c)
	}

	tmpl := server.MakeTmpl(StaticFs, "Data")
	tmpl["Base"] = srvConfig.Config.CHESSMetaData.WebServer.Base
	tmpl["User"] = user
	tmpl["Date"] = time.Now().Unix()
	tmpl["Beamlines"] = _beamlines
	var forms []string
	for idx, fname := range srvConfig.Config.CHESSMetaData.SchemaFiles {
		cls := "hide"
		if idx == 0 {
			cls = ""
		}
		form, err := genForm(c, fname, nil)
		if err != nil {
			msg := "could not parse http form"
			handleError(c, http.StatusInternalServerError, msg, err)
			return
		}
		beamlineForm := fmt.Sprintf("<div id=\"%s\" class=\"%s\">%s</div>", utils.FileName(fname), cls, form)
		forms = append(forms, beamlineForm)
	}
	tmpl["Form"] = template.HTML(strings.Join(forms, "\n"))
	page := server.TmplPage(StaticFs, "metaforms.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+page+footer()))
}

// ProvenanceHandler provides access to GET /provenance endpoint
func ProvenanceHandler(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+"Not Implemented"+footer()))
}

// AIMLHandler provides access to GET /aiml endpoint
func AIMLHandler(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+"Not Implemented"+footer()))
}

// AnalysisHandler provides access to GET /analysis endpoint
func AnalysisHandler(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+"Not Implemented"+footer()))
}

// VisualizationHandler provides access to GET /visualization endpoint
func VisualizationHandler(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+"Not Implemented"+footer()))
}

// DataHandler provides access to GET /data endpoint
func DataHandler(c *gin.Context) {
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
	if Verbose > 0 {
		log.Printf("INFO: Authz response %+v, error %v", response, err)
	}
	if response.Status != "ok" {
		msg := fmt.Sprintf("No user %s found in Authz service", form.User)
		content = errorTmpl(c, msg, errors.New("user not found"))
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}

	c.Set("user", form.User)
	if Verbose > 0 {
		log.Printf("login from user %s, url path %s", form.User, c.Request.URL.Path)
	}

	// set our user cookie
	if _, err := c.Cookie("user"); err != nil {
		if Verbose > 0 {
			log.Printf("set cookie user=%s domain=%s", form.User, domain())
		}
		c.SetCookie("user", form.User, 3600, "/", domain(), false, true)
	}

	// redirect
	c.Redirect(http.StatusFound, "/")
}

// UploadJsonHandler handles upload of JSON record
func UploadJsonHandler(c *gin.Context) {

	user, err := c.Cookie("user")
	if err != nil {
		LoginHandler(c)
	}

	r := c.Request
	w := c.Writer
	// get beamline value from the form
	sname := r.FormValue("SchemaName")

	// read form file
	file, _, err := r.FormFile("file")
	if err != nil {
		msg := "unable to read file form"
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}
	defer file.Close()

	defer r.Body.Close()
	body, err := ioutil.ReadAll(file)
	var rec mongo.Record
	if err == nil {
		err = json.Unmarshal(body, &rec)
		if err != nil {
			log.Println("unable to read HTTP JSON record, error:", err)
		}
	}
	tmpl := server.MakeTmpl(StaticFs, "Upload")
	tmpl["User"] = user
	tmpl["Date"] = time.Now().Unix()
	schemaFiles := srvConfig.Config.CHESSMetaData.SchemaFiles
	if sname != "" {
		// construct proper schema files order which will be used to generate forms
		sfiles := []string{}
		// add scheme file which matches our desired schema
		for _, f := range schemaFiles {
			if strings.Contains(f, sname) {
				sfiles = append(sfiles, f)
			}
		}
		// add rest of schema files
		for _, f := range schemaFiles {
			if !strings.Contains(f, sname) {
				sfiles = append(sfiles, f)
			}
		}
		schemaFiles = sfiles
		// construct proper bemalines order
		blines := []string{sname}
		for _, b := range _beamlines {
			if b != sname {
				blines = append(blines, b)
			}
		}
		tmpl["Beamlines"] = blines
	} else {
		tmpl["Beamlines"] = _beamlines
	}
	var forms []string
	for idx, fname := range schemaFiles {
		cls := "hide"
		if idx == 0 {
			cls = ""
		}
		form, err := genForm(c, fname, &rec)
		if err != nil {
			log.Println("ERROR", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		beamlineForm := fmt.Sprintf("<div id=\"%s\" class=\"%s\">%s</div>", utils.FileName(fname), cls, form)
		forms = append(forms, beamlineForm)
	}
	tmpl["Form"] = template.HTML(strings.Join(forms, "\n"))
	page := server.TmplPage(StaticFs, "metaforms.tmpl", tmpl)
	w.Write([]byte(header() + page + footer()))
}
