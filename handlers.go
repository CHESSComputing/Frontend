package main

// handlers module
//
// Copyright (c) 2023 - Valentin Kuznetsov <vkuznet@gmail.com>
//
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
	"strconv"
	"strings"
	"time"

	authz "github.com/CHESSComputing/golib/authz"
	beamlines "github.com/CHESSComputing/golib/beamlines"
	srvConfig "github.com/CHESSComputing/golib/config"
	server "github.com/CHESSComputing/golib/server"
	services "github.com/CHESSComputing/golib/services"
	utils "github.com/CHESSComputing/golib/utils"
	"github.com/gin-gonic/gin"
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

//
// OAuth handlers
//

// GithubOauthLoginHandler provides kerberos authentication handler
func GithubOauthLoginHandler(c *gin.Context) {
	authz.GithubOauthLogin(c, Verbose)
}

// GithubCallBackHandler provides kerberos authentication handler
func GithubCallBackHandler(c *gin.Context) {
	authz.GithubCallBack(c, "/services", Verbose)
}

// GoogleOauthLoginHandler provides kerberos authentication handler
func GoogleOauthLoginHandler(c *gin.Context) {
	authz.GoogleOauthLogin(c, Verbose)
}

// GoogleCallBackHandler provides kerberos authentication handler
func GoogleCallBackHandler(c *gin.Context) {
	authz.GoogleCallBack(c, "/services", Verbose)
}

// FacebookOauthLoginHandler provides kerberos authentication handler
func FacebookOauthLoginHandler(c *gin.Context) {
	authz.FacebookOauthLogin(c, Verbose)
}

// FacebookCallBackHandler provides kerberos authentication handler
func FacebookCallBackHandler(c *gin.Context) {
	authz.FacebookCallBack(c, "/services", Verbose)
}

//
// GET handlers
//

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

// NotImplementedHandler provides access to GET /login endpoint
func NotImplementedHandler(c *gin.Context) {
	tmpl := server.MakeTmpl(StaticFs, "Login")
	base := srvConfig.Config.Frontend.WebServer.Base
	tmpl["Base"] = base
	tmpl["Content"] = "Not implemented"
	content := server.TmplPage(StaticFs, "error.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// LoginHandler provides access to GET /login endpoint
func LoginHandler(c *gin.Context) {
	tmpl := server.MakeTmpl(StaticFs, "Login")
	base := srvConfig.Config.Frontend.WebServer.Base
	tmpl["Base"] = base
	// add oauth login buttons
	tmpl["GithubLogin"] = ""
	tmpl["GoogleLogin"] = ""
	tmpl["FacebookLogin"] = ""
	for _, arec := range srvConfig.Config.Frontend.OAuth {
		if arec.Provider == "github" {
			tmpl["GithubLogin"] = fmt.Sprintf("%s/github/login", base)
		} else if arec.Provider == "google" {
			tmpl["GoogleLogin"] = fmt.Sprintf("%s/google/login", base)
		} else if arec.Provider == "facebook" {
			tmpl["FacebookLogin"] = fmt.Sprintf("%s/facebook/login", base)
		}
	}
	content := server.TmplPage(StaticFs, "login.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// LogoutHandler provides access to GET /logout endpoint
func LogoutHandler(c *gin.Context) {
	c.SetCookie("user", "", -1, "/", utils.Domain(), false, true)
	cookie := &http.Cookie{
		Name:     "user",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
	}
	http.SetCookie(c.Writer, cookie)
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
	content, err := server.MDToHTML(StaticFs, fname)
	if err != nil {
		content = fmt.Sprintf("unable to convert %s to HTML, error %v", fname, err)
		log.Println("ERROR: ", content)
		tmpl["Content"] = content
	}
	tmpl["Content"] = template.HTML(content)
	content = server.TmplPage(StaticFs, "content.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// DBSFilesHandler provides access to GET /meta/files endpoint
func DBSFilesHandler(c *gin.Context) {
	r := c.Request
	did := r.FormValue("did")

	// obtain valid token
	_httpReadRequest.GetToken()

	rec := services.ServiceRequest{
		Client:       "frontend",
		ServiceQuery: services.ServiceQuery{},
	}
	data, err := json.Marshal(rec)
	if err != nil {
		msg := "unable to parse user query"
		handleError(c, http.StatusInternalServerError, msg, err)
		return
	}

	// search request to DataDiscovery service
	rurl := fmt.Sprintf("%s/files?did=%s", srvConfig.Config.Services.DataBookkeepingURL, did)
	resp, err := _httpReadRequest.Get(rurl)
	if err != nil {
		msg := "unable to get meta-data from upstream server"
		handleError(c, http.StatusInternalServerError, msg, err)
		return
	}
	// parse data records from meta-data service
	defer resp.Body.Close()
	data, err = io.ReadAll(resp.Body)
	if err != nil {
		content := errorTmpl(c, "unable to read response body, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}
	if Verbose > 0 {
		log.Println("dbs data\n", string(data))
	}
	var records []map[string]any
	err = json.Unmarshal(data, &records)
	if err != nil {
		content := errorTmpl(c, "unable to unmarshal dbs data, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}
	var files []string
	for _, r := range records {
		if f, ok := r["file"]; ok {
			fname := f.(string)
			files = append(files, fname)
		}
	}
	tmpl := server.MakeTmpl(StaticFs, "DBS Files")
	tmpl["Files"] = strings.Join(files, "\n")
	tmpl["Data"] = string(data)
	tmpl["Did"] = did
	page := server.TmplPage(StaticFs, "dbs_files.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+page+footer()))
}

// ToolsHandler provides access to GET /tools endpoint
func ToolsHandler(c *gin.Context) {
	tmpl := server.MakeTmpl(StaticFs, "Tools")
	base := srvConfig.Config.Frontend.WebServer.Base
	tmpl["Base"] = base
	content := server.TmplPage(StaticFs, "tools.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// SearchHandler provides access to GET /search endpoint
func SearchHandler(c *gin.Context) {
	r := c.Request
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
	if qlkeys, err := json.Marshal(qlKeys()); err == nil {
		tmpl["QLKeys"] = string(qlkeys)
	} else {
		tmpl["QLKeys"] = []string{}
	}

	// if we got GET request it is /search web form without query request
	if r.Method == "GET" && r.FormValue("query") == "" {
		page := server.TmplPage(StaticFs, "searchform.tmpl", tmpl)
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+page+footer()))
		return
	}

	// if we get POST request we'll process user query
	query := r.FormValue("query")
	if Verbose > 0 {
		log.Printf("search query='%s' user=%v", query, user)
	}
	if err != nil {
		msg := "unable to parse user query"
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}

	// obtain valid token
	_httpReadRequest.GetToken()

	// create POST payload
	var idx, limit int
	idxStr := r.FormValue("idx")
	if idxStr != "" {
		idx, err = strconv.Atoi(idxStr)
		log.Println("idx", idx, err)
	}
	limitStr := r.FormValue("limit")
	if limitStr != "" {
		limit, err = strconv.Atoi(limitStr)
		log.Println("limit", limit, err)
	}
	if limit == 0 {
		limit = -1
	}
	rec := services.ServiceRequest{
		Client:       "frontend",
		ServiceQuery: services.ServiceQuery{Query: query, Idx: idx, Limit: limit},
	}
	log.Printf("service request record\n%s", rec.String())
	data, err := json.Marshal(rec)
	if err != nil {
		msg := "unable to parse user query"
		handleError(c, http.StatusInternalServerError, msg, err)
		return
	}

	// search request to DataDiscovery service
	rurl := fmt.Sprintf("%s/search", srvConfig.Config.Services.DiscoveryURL)
	resp, err := _httpReadRequest.Post(rurl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		msg := "unable to get meta-data from upstream server"
		handleError(c, http.StatusInternalServerError, msg, err)
		return
	}
	// parse data records from meta-data service
	var response services.ServiceResponse
	defer resp.Body.Close()
	data, err = io.ReadAll(resp.Body)
	if err != nil {
		content := errorTmpl(c, "unable to read response body, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}
	err = json.Unmarshal(data, &response)
	if err != nil {
		content := errorTmpl(c, "unable to unmarshal response, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}
	if Verbose > 1 {
		log.Printf("meta-data response\n%+v", response)
	}
	records := response.Results.Records
	nrecords := response.Results.NRecords
	content := records2html(user, records)
	tmpl["Records"] = template.HTML(content)

	pages := pagination(c, query, nrecords, idx, limit)
	tmpl["Pagination"] = template.HTML(pages)

	page := server.TmplPage(StaticFs, "records.tmpl", tmpl)
	// we will not use footer() on handlers since user may expand records
	// instead we'll use footerEmpty() function
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+page+footerEmpty()))
	// c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+page+footer()))
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

// helper function to parse metadata upload form using user's provided file
func parseFileUploadForm(c *gin.Context) (services.MetaRecord, error) {
	r := c.Request
	mrec := services.MetaRecord{}
	user, _ := c.Cookie("user")

	// read schema name from web form
	var schema string
	sname := r.FormValue("SchemaName")
	mrec.Schema = sname
	if sname != "" {
		schema = beamlines.SchemaFileName(sname)
	}
	if sname == "" {
		msg := "client does not provide schema name"
		return mrec, errors.New(msg)
	}
	if Verbose > 0 {
		log.Printf("schema=%s, file=%s", sname, schema)
	}

	// process web form

	file, _, err := r.FormFile("file")
	if err != nil {
		return mrec, err
	}
	defer file.Close()
	body, err := io.ReadAll(file)
	var rec map[string]any
	err = json.Unmarshal(body, &rec)
	rec["User"] = user
	mrec.Record = rec
	return mrec, err
}

// helper function to parse meta upload web form
func parseFormUploadForm(c *gin.Context) (services.MetaRecord, error) {
	r := c.Request
	log.Printf("### parseFormUploadForm %+v", r)
	mrec := services.MetaRecord{}
	user, _ := c.Cookie("user")
	// read schemaName from form beamlines drop-down
	//     sname := r.FormValue("beamlines")
	sname := r.FormValue("SchemaName")
	log.Println("### parseFormUploadForm", sname, user)
	mrec.Schema = sname
	fname := beamlines.SchemaFileName(sname)
	schema, err := _smgr.Load(fname)
	if err != nil {
		log.Println("ERROR", err)
		return mrec, err
	}
	desc := ""
	// r.PostForm provides url.Values which is map[string][]string type
	// we convert it to Record
	rec := make(map[string]any)
	for k, vals := range r.PostForm {
		items := utils.UniqueFormValues(vals)
		if Verbose > 0 {
			log.Printf("### PostForm key=%s items=%v type(items)=%T", k, items, items)
		}
		if k == "SchemaName" {
			continue
		}
		if k == "Description" {
			desc = strings.Join(items, " ")
			continue
		}
		val, err := parseValue(schema, k, items)
		if err != nil {
			// check if given key is mandatory or optional
			srec, ok := schema.Map[k]
			if ok {
				if srec.Optional {
					log.Println("WARNING: unable to parse optional key", k)
				} else {
					log.Println("ERROR: unable to parse mandatory key", k, "error", err)
					return mrec, err
				}
			} else {
				if !utils.InList(k, beamlines.SkipKeys) {
					log.Println("ERROR: no key", k, "found in schema map, error", err)
					return mrec, err
				}
			}
		}
		rec[k] = val
	}
	// create did from the form upload
	attrs := srvConfig.Config.DID.Attributes
	sep := srvConfig.Config.DID.Separator
	div := srvConfig.Config.DID.Divider
	did := utils.CreateDID(rec, attrs, sep, div)
	rec["did"] = did
	rec["User"] = user
	rec["Description"] = desc
	if Verbose > 0 {
		log.Printf("process form, record %v\n", rec)
	}
	mrec.Record = rec
	return mrec, nil
}

// MetaFormUploadHandler provides access to GET /meta/form/upload endpoint
func MetaFormUploadHandler(c *gin.Context) {
	rec, err := parseFormUploadForm(c)
	if err != nil {
		handleError(c, http.StatusBadRequest, "unable to parse file upload form", err)
		return
	}
	log.Println("### form record", rec.JsonString())
	MetaUploadHandler(c, rec)
}

// MetaFileUploadHandler provides access to GET /meta/file/upload endpoint
func MetaFileUploadHandler(c *gin.Context) {
	rec, err := parseFileUploadForm(c)
	if err != nil {
		handleError(c, http.StatusBadRequest, "unable to parse file upload form", err)
		return
	}
	MetaUploadHandler(c, rec)
}

// MetaUploadHandler manages upload of record to MetaData service
func MetaUploadHandler(c *gin.Context, mrec services.MetaRecord) {
	user, err := c.Cookie("user")
	if err != nil {
		LoginHandler(c)
	}
	tmpl := server.MakeTmpl(StaticFs, "Upload")

	// prepare http writer
	_httpWriteRequest.GetToken()

	// place request to MetaData service
	rurl := fmt.Sprintf("%s", srvConfig.Config.Services.MetaDataURL)
	data, err := json.MarshalIndent(mrec, "", "  ")
	if err != nil {
		content := errorTmpl(c, "unable to marshal meta data record, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}
	tmpl["JsonRecord"] = template.HTML(string(data))
	resp, err := _httpWriteRequest.Post(rurl, "application/json", bytes.NewBuffer(data))
	class := "alert alert-success"
	msg := fmt.Sprintf("Your meta-data is inserted successfully")
	if err != nil {
		class = "alert alert-error"
		msg = fmt.Sprintf("meta-data request processing error: %v", err)
	}
	defer resp.Body.Close()
	data, err = io.ReadAll(resp.Body)
	if err != nil {
		class = "alert alert-error"
		msg = fmt.Sprintf("read response error: %v", err)
	}

	var sresp services.ServiceResponse
	err = json.Unmarshal(data, &sresp)
	if err != nil {
		class = "alert alert-error"
		msg = fmt.Sprintf("read response error: %v", err)
	}
	if sresp.SrvCode != 0 || sresp.HttpCode != http.StatusOK {
		msg = fmt.Sprintf("<pre>%s<pre>", sresp.String())
	}

	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	tmpl["User"] = user
	tmpl["Date"] = time.Now().Unix()
	tmpl["Schema"] = mrec.Schema
	tmpl["Message"] = msg
	tmpl["Status"] = sresp.Status
	tmpl["Class"] = class
	tmpl["ResponseRecord"] = template.HTML(sresp.JsonString())
	content := server.TmplPage(StaticFs, "upload_status.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// ProvenanceHandler provides access to GET /provenance endpoint
func ProvenanceHandler(c *gin.Context) {
	tmpl := server.MakeTmpl(StaticFs, "Provenance")
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	content := server.TmplPage(StaticFs, "provenance.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// SpecScansHandler provides access to GET /scanspecs endpoint
func SpecScansHandler(c *gin.Context) {
	tmpl := server.MakeTmpl(StaticFs, "SpecScans")
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	content := server.TmplPage(StaticFs, "specscans.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// NotebookHandler provides access to GET /notebook endpoint
func NotebookHandler(c *gin.Context) {
	chapbookUrl := fmt.Sprintf("%s/notebook", srvConfig.Config.Services.CHAPBookURL)
	if chapbookUrl != "" {
		if c.Request.Header.Get("Authorization") == "" {
			user, _ := c.Cookie("user")
			token, err := newToken(user, "read")
			if err == nil {
				// pass token as paramter to CHAPBook /notebook end-point
				// since HTTP standard does not pass through HTTP headers on redirect
				// see discussion: https://stackoverflow.com/questions/36345696/http-redirect-with-headers
				chapbookUrl = fmt.Sprintf("%s?token=%s", chapbookUrl, token)
			}
		}
		c.Redirect(http.StatusFound, chapbookUrl)
		return
	}
	tmpl := server.MakeTmpl(StaticFs, "Notebook")
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	content := server.TmplPage(StaticFs, "notebook.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// PublishHandler provides access to GET /piublish endpoint
func PublishHandler(c *gin.Context) {
	tmpl := server.MakeTmpl(StaticFs, "Publication Service")
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	content := server.TmplPage(StaticFs, "publish.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// AIMLHandler provides access to GET /aiml endpoint
func AIMLHandler(c *gin.Context) {
	tmpl := server.MakeTmpl(StaticFs, "AI/ML")
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	content := server.TmplPage(StaticFs, "ai_ml.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// AnalysisHandler provides access to GET /analysis endpoint
func AnalysisHandler(c *gin.Context) {
	tmpl := server.MakeTmpl(StaticFs, "Data Analysis")
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	content := server.TmplPage(StaticFs, "data_analysis.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// VisualizationHandler provides access to GET /visualization endpoint
func VisualizationHandler(c *gin.Context) {
	tmpl := server.MakeTmpl(StaticFs, "Vizualization")
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	content := server.TmplPage(StaticFs, "visualization.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// DataHandler provides access to GET /data endpoint
func DataHandler(c *gin.Context) {
	tmpl := server.MakeTmpl(StaticFs, "Data Management")
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	content := server.TmplPage(StaticFs, "data_management.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// POST handlers

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
	var rec map[string]any
	if err == nil {
		err = json.Unmarshal(body, &rec)
		if err != nil {
			log.Println("unable to read HTTP JSON record, error:", err)
		}
	}
	tmpl := server.MakeTmpl(StaticFs, "Upload")
	tmpl["User"] = user
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
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
