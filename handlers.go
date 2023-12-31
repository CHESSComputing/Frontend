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
