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
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	authz "github.com/CHESSComputing/golib/authz"
	beamlines "github.com/CHESSComputing/golib/beamlines"
	srvConfig "github.com/CHESSComputing/golib/config"
	"github.com/CHESSComputing/golib/ql"
	server "github.com/CHESSComputing/golib/server"
	services "github.com/CHESSComputing/golib/services"
	utils "github.com/CHESSComputing/golib/utils"
	"github.com/gin-gonic/gin"
	"gopkg.in/jcmturner/gokrb5.v7/credentials"
)

// Documentation about gib handlers can be found over here:
// https://go.dev/doc/tutorial/web-service-gin

var DEFAULT_END_POINT string

//
// Data structure we use through the code
//

// DocsParams represents URI storage params in /docs/:page end-point
type DocsParams struct {
	Page string `uri:"page" binding:"required"`
}

// MetaParams represents /record?did=bla end-point
type MetaParams struct {
	DID string `form:"did"`
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
	authz.GithubCallBack(c, DEFAULT_END_POINT, Verbose)
}

// GoogleOauthLoginHandler provides kerberos authentication handler
func GoogleOauthLoginHandler(c *gin.Context) {
	authz.GoogleOauthLogin(c, Verbose)
}

// GoogleCallBackHandler provides kerberos authentication handler
func GoogleCallBackHandler(c *gin.Context) {
	authz.GoogleCallBack(c, DEFAULT_END_POINT, Verbose)
}

// FacebookOauthLoginHandler provides kerberos authentication handler
func FacebookOauthLoginHandler(c *gin.Context) {
	authz.FacebookOauthLogin(c, Verbose)
}

// FacebookCallBackHandler provides kerberos authentication handler
func FacebookCallBackHandler(c *gin.Context) {
	authz.FacebookCallBack(c, DEFAULT_END_POINT, Verbose)
}

// helper function to get user from gin context
func getUser(c *gin.Context) (string, error) {
	var user string
	var err error
	if srvConfig.Config.Frontend.TestMode {
		user = "test"
	} else {
		user, err = c.Cookie("user")
	}
	return user, err
}

//
// GET handlers
//

// KAuthHandler provides kerberos authentication handler
func KAuthHandler(c *gin.Context) {
	// get http request/writer
	w := c.Writer
	r := c.Request

	//     user, err := c.Cookie("user")
	user, err := getUser(c)
	if err == nil && user != "" {
		log.Println("found user cookie", user)
		c.Redirect(http.StatusFound, "/dstable")
		return
	}

	expiration := time.Now().Add(24 * time.Hour)
	// in test mode we'll set user as TestUser
	if srvConfig.Config.Frontend.TestMode {
		log.Println("frontend test mode")
		c.Set("user", "TestUser")
		cookie := http.Cookie{Name: "user", Value: "TestUser", Expires: expiration}
		http.SetCookie(w, &cookie)
		c.Redirect(http.StatusFound, DEFAULT_END_POINT)
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
	c.Redirect(http.StatusFound, DEFAULT_END_POINT)
}

// MainHandler provides access to GET / end-point
func MainHandler(c *gin.Context) {
	// check if user cookie is set, this is necessary as we do not
	// use authorization handler for / end-point
	user, err := getUser(c)
	if err == nil {
		c.Set("user", user)
		// switch to default handler
		DatasetsTableHandler(c)
		//         ServicesHandler(c)
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
	user, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
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
	if srvConfig.Config.Frontend.DocUrl != "" {
		c.Redirect(http.StatusFound, srvConfig.Config.Frontend.DocUrl)
		return
	}
	DocsLocalHandler(c)
}

// DocsLocalHandler provides access to GET /docs end-point
func DocsLocalHandler(c *gin.Context) {
	// check if user cookie is set, this is necessary as we do not
	// use authorization handler for /docs end-point
	tmpl := server.MakeTmpl(StaticFs, "Documentation")
	tmpl["Title"] = "Documentation"
	fname := "static/markdown/main.md"
	var params DocsParams
	if err := c.ShouldBindUri(&params); err == nil {
		if strings.HasSuffix(params.Page, "md") {
			fname = fmt.Sprintf("static/markdown/%s", params.Page)
		} else if strings.HasSuffix(params.Page, "pdf") {
			fname = fmt.Sprintf("/media/%s", params.Page)
			c.Redirect(http.StatusFound, fname)
			return
		} else {
			content := fmt.Sprintf("no suitable file found for %s", fname)
			tmpl["Content"] = content
			content = server.TmplPage(StaticFs, "content.tmpl", tmpl)
			c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
			return
		}
	}
	content, err := server.MDToHTML(StaticFs, fname)
	if err != nil {
		content = fmt.Sprintf("unable to convert %s to HTML, error %v", fname, err)
		log.Println("ERROR: ", content)
		tmpl["Content"] = content
	} else {
		tmpl["Content"] = template.HTML(content)
	}
	content = server.TmplPage(StaticFs, "content.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// ProvenanceHandler provides access to GET /provenance endpoint
func ProvenanceHandler(c *gin.Context) {
	r := c.Request
	did := r.FormValue("did") // extract did from post form or from /provenance?did=did

	// obtain valid token
	_httpReadRequest.GetToken()

	// get files from provenance service
	records, err := getData("files", did)
	if err != nil {
		content := errorTmpl(c, "unable to get files data from provenance service, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}
	var inputFiles, outputFiles []string
	for _, r := range records {
		if f, ok := r["name"]; ok {
			fname := f.(string)
			if t, ok := r["file_type"]; ok {
				v := t.(string)
				if v == "input" {
					inputFiles = append(inputFiles, fname)
				} else if v == "output" {
					outputFiles = append(outputFiles, fname)
				}
			}
		}
	}
	// get files from provenance service
	var parents []string
	records, err = getData("parents", did)
	if err != nil {
		content := errorTmpl(c, "unable to get parents data from provenance service, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}
	for _, r := range records {
		if f, ok := r["parent_did"]; ok {
			if f != nil {
				v := f.(string)
				parents = append(parents, v)
			}
		}
	}
	// get children from provenance service
	var children []string
	records, err = getData("child", did)
	if err != nil {
		content := errorTmpl(c, "unable to get child data from provenance service, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}
	for _, r := range records {
		if f, ok := r["child_did"]; ok {
			if f != nil {
				v := f.(string)
				children = append(children, v)
			}
		}
	}

	// obtain provenance record
	provenance, err := getData("provenance", did)
	if err != nil {
		content := errorTmpl(c, "unable to get provenance record from provenance service, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}

	// construct output record
	tmpl := server.MakeTmpl(StaticFs, "Provenance information")
	tmpl["InputFiles"] = strings.Join(inputFiles, "\n")
	tmpl["OutputFiles"] = strings.Join(outputFiles, "\n")
	if len(parents) > 0 {
		tmpl["Parents"] = strings.Join(parents, "\n")
	} else {
		tmpl["Parents"] = "Not available"
	}
	if len(children) > 0 {
		tmpl["Children"] = strings.Join(children, "\n")
	} else {
		tmpl["Children"] = "Not available"
	}
	tmpl["Provenance"] = "Not available"
	if len(provenance) > 0 {
		if data, err := json.MarshalIndent(provenance, "", "  "); err == nil {
			tmpl["Provenance"] = string(data)
		} else {
			log.Println("ERROR: unable to marshal provenance records")
		}
	}
	tmpl["Did"] = did
	// fill out necessary aux info
	for _, key := range []string{"osinfo", "environments", "scripts", "packages"} {
		records, err = getData(key, did)
		if err != nil {
			content := errorTmpl(c, fmt.Sprintf("unable to get %s data from provenance service, error", key), err)
			c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
			return
		}
		if data, err := json.MarshalIndent(records, "", "  "); err == nil {
			tmpl[key] = string(data)
		}
	}

	page := server.TmplPage(StaticFs, "provenance.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+page+footer()))
}

// DMFiles provides access to GET /dm end-point
func DMFilesHandler(c *gin.Context) {
	user, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
	}
	c.Set("user", user)
	ext := c.Request.FormValue("ext")
	did := c.Request.FormValue("did")
	if did == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'did' parameter"})
		return
	}
	did = url.QueryEscape(did)
	pat := url.QueryEscape(ext)

	// Prepare redirection URL
	targetURL := fmt.Sprintf("%s/files?did=%s&pattern=%s", srvConfig.Config.DataManagementURL, did, pat)

	// get new read token
	token, err := newToken(user, "read")
	if err != nil {
		handleError(c, http.StatusBadRequest, "unable to acquire read token", err)
		return
	}

	// Create a new HTTP request to the target URL
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		handleError(c, http.StatusBadRequest, "unable to query DataManagement service", err)
		return
	}

	// Set custom headers
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Custom-Header", "DataManagementRequest")

	// Copy headers from the original request
	for key, values := range c.Request.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		handleError(c, http.StatusBadRequest, "unable to query DataManagement service", err)
		return
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		handleError(c, http.StatusBadRequest, "unable to read the data", err)
		return
	}

	var files []string
	err = json.Unmarshal(data, &files)
	if err != nil {
		handleError(c, http.StatusBadRequest, "unable to unmarshal the data", err)
		return
	}
	if c.Request.Header.Get("Accept") == "application/json" {
		c.JSON(http.StatusOK, files)
		return
	}
	content := "<section><article>"
	if len(files) == 0 {
		content = "No files found for your pattern"
	} else {
		if val, err := url.QueryUnescape(did); err == nil {
			did = val
		}
		if val, err := url.QueryUnescape(pat); err == nil {
			pat = val
		}
		content = fmt.Sprintf("%s<h4>DID: %s</h4>", content, did)
		for _, f := range files {
			content = fmt.Sprintf("%s\n<br/>%s", content, f)
		}
	}
	content += "</article></section>"
	c.Writer.Write([]byte(header() + content + footerEmpty()))
}

// DataManagementHandler provides access to GET /dm end-point
func DataManagementHandler(c *gin.Context) {
	// check if user cookie is set, this is necessary as we do not
	// use authorization handler for / end-point
	user, err := getUser(c)
	if err == nil {
		c.Set("user", user)

		path := c.Query("path")
		fname := c.Query("file")
		did := c.Query("did")
		if did == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'did' parameter"})
			return
		}
		did = url.QueryEscape(did)

		// Prepare redirection URL
		targetURL := fmt.Sprintf("%s/data?did=%s", srvConfig.Config.DataManagementURL, did)
		if path != "" {
			targetURL = fmt.Sprintf("%s&path=%s", targetURL, url.QueryEscape(path))
		}
		if fname != "" {
			targetURL = fmt.Sprintf("%s&file=%s", targetURL, url.QueryEscape(fname))
		}

		// get new read token
		token, err := newToken(user, "read")
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		// Create a new HTTP request to the target URL
		req, err := http.NewRequest(http.MethodGet, targetURL, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create request"})
			return
		}

		// Set custom headers
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Custom-Header", "DataManagementRequest")

		// Copy headers from the original request
		for key, values := range c.Request.Header {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}

		// Send the request
		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to forward request"})
			return
		}
		defer resp.Body.Close()

		// Copy response headers
		for key, values := range resp.Header {
			for _, value := range values {
				c.Writer.Header().Add(key, value)
			}
		}

		// Set response status code
		c.Status(resp.StatusCode)

		// Copy response body to Gin's response writer
		io.Copy(c.Writer, resp.Body)

	} else {
		LoginHandler(c)
	}
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
	user, err := getUser(c)
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
	qlRecords, err := ql.QLRecords("")
	qlkeys := []string{}
	for _, qrec := range qlRecords {
		srv := fmt.Sprintf("%s:%s", qrec.Service, qrec.Schema)
		if qrec.Schema == "" {
			srv = qrec.Service
		}
		out := fmt.Sprintf("%s: (%s) %s, units:%s, data-type:%s",
			qrec.Key, srv, qrec.Description, qrec.Units, qrec.DataType)
		qlkeys = append(qlkeys, out)
	}
	if err != nil {
		log.Println("ERROR", err)
		tmpl["QLKeys"] = []string{}
	} else {
		if val, err := json.Marshal(qlkeys); err == nil {
			tmpl["QLKeys"] = string(val)
		} else {
			tmpl["QLKeys"] = []string{}
		}
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
	// first check if web form provides fix query input
	fix := r.FormValue("fix")
	if fix == "true" {
		tmpl["FixQuery"] = query
		page := server.TmplPage(StaticFs, "searchform.tmpl", tmpl)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+page+footer()))
		return
	}
	// proceed with processing the user query from web form
	if query == "" {
		msg := "Empty query"
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}
	dataTypes := []string{"STRING", "INT", "INTEGER", "FLOAT", "LIST", "BOOL"}
	for _, key := range dataTypes {
		if strings.Contains(query, key) {
			tmpl := server.MakeTmpl(StaticFs, "Data")
			tmpl["Base"] = srvConfig.Config.CHESSMetaData.WebServer.Base
			tmpl["Query"] = query
			tmpl["Key"] = key
			page := server.TmplPage(StaticFs, "query_error.tmpl", tmpl)
			msg := string(template.HTML(page))
			handleError(c, http.StatusBadRequest, msg, err)
			return
		}
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
		limit = 10
	}
	// parse sort keys which are provided as comma separated list
	sortKeys := r.FormValue("sort_keys")
	var skeys []string
	if sortKeys != "" {
		for _, k := range strings.Split(sortKeys, ",") {
			if strings.Contains(k, "-ascending") {
				k = strings.Replace(k, "-ascending", "", -1)
			} else if strings.Contains(k, "-descending") {
				k = strings.Replace(k, "-descending", "", -1)
			}
			skeys = append(skeys, k)
		}
	}
	// use date as default sort key
	if len(skeys) == 0 {
		skeys = append(skeys, "date")
	}
	sortOrder := r.FormValue("sort_order")
	order := -1 // descending order for MongoDB (default value)
	if sortOrder != "" {
		// in pagination.tmpl we use ascending/descending which we translates to 1/-1 for MongoDB
		if sortOrder == "ascending" || sortOrder == "asc" {
			order = 1
		} else if sortOrder == "descending" || sortOrder == "des" || sortOrder == "desc" {
			order = -1
		} else {
			order, err = strconv.Atoi(sortOrder)
			if err != nil {
				log.Println("ERROR: unable to decode sort order, error:", err)
				order = -1 // default value
			}
		}
	}
	rec := services.ServiceRequest{
		Client:       "frontend",
		ServiceQuery: services.ServiceQuery{Query: query, Idx: idx, Limit: limit, SortKeys: skeys, SortOrder: order},
	}
	// request only user's specific data (check user attributes)
	if user != "test" && srvConfig.Config.Frontend.CheckBtrs && srvConfig.Config.Embed.DocDb == "" {
		attrs, err := chessAttributes(user)
		if err == nil {
			var spec map[string]any
			err := json.Unmarshal([]byte(query), &spec)
			if err != nil {
				msg := fmt.Sprintf("malformed query %+v, unable to create spec", query)
				handleError(c, http.StatusBadRequest, msg, err)
				return
			}
			// in search we only update spec with user's btrs
			spec = updateSpec(spec, attrs, "search")
			if data, err := json.Marshal(spec); err == nil {
				query = string(data)
			}
			rec = services.ServiceRequest{
				Client: "frontend",
				ServiceQuery: services.ServiceQuery{
					Query:     query,
					Spec:      spec,
					SortKeys:  skeys,
					SortOrder: order,
					Idx:       idx,
					Limit:     limit,
				},
			}
		}
	}
	// based on user query process request from all FOXDEN services
	processResults(c, rec, user, idx, limit)
}

// SpecScansHandler provides information about spec scan records
func SpecScansHandler(c *gin.Context) {
	_, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
	}

	var params MetaParams
	err = c.Bind(&params)
	if err != nil {
		rec := services.Response("MetaData", http.StatusBadRequest, services.BindError, err)
		c.JSON(http.StatusBadRequest, rec)
		return
	}
	did := params.DID
	//     query := fmt.Sprintf("{\"did\": \"%s\"}", did)
	rec := services.ServiceRequest{
		Client:       "foxden",
		ServiceQuery: services.ServiceQuery{Query: did, Idx: 0, Limit: -1},
	}

	// parse response from SpecScan service to show its records
	data, err := json.Marshal(rec)
	rurl := fmt.Sprintf("%s/search", srvConfig.Config.Services.SpecScansURL)
	resp, err := _httpReadRequest.Post(rurl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		rec := services.Response("Frontend", http.StatusBadRequest, services.BindError, err)
		c.JSON(http.StatusBadRequest, rec)
		return
	}
	defer resp.Body.Close()
	data, err = io.ReadAll(resp.Body)
	if err != nil {
		rec := services.Response("Frontend", http.StatusBadRequest, services.BindError, err)
		c.JSON(http.StatusBadRequest, rec)
		return
	}
	//         var response services.ServiceResponse
	//         err = json.Unmarshal(data, &response)
	c.Data(http.StatusOK, "application/json", data)
}

// MetaDataHandler provides access to GET /meta endpoint
func MetaDataHandler(c *gin.Context) {
	user, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
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
	user, _ := getUser(c)

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
	rec["user"] = user
	mrec.Record = rec
	return mrec, err
}

// helper function to parse meta upload web form
func parseFormUploadForm(c *gin.Context) (services.MetaRecord, error) {
	r := c.Request
	mrec := services.MetaRecord{}
	user, _ := getUser(c)
	// read schemaName from form beamlines drop-down
	//     sname := r.FormValue("beamlines")
	sname := r.FormValue("SchemaName")
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
		if k == "User" {
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
	val, _ := rec["did"]
	recDid := fmt.Sprintf("%v", val)
	if recDid == "" {
		did := utils.CreateDID(rec, attrs, sep, div)
		rec["did"] = did
	}
	rec["user"] = user
	rec["description"] = desc
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
	user, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
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

// ProvInfoHandler provides access to GET /provinfo endpoint
func ProvInfoHandler(c *gin.Context) {
	tmpl := server.MakeTmpl(StaticFs, "Provenance information")
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	content := server.TmplPage(StaticFs, "provinfo.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// SpecScansHandler provides access to GET /scanspecs endpoint
// func SpecScansHandler(c *gin.Context) {
//     tmpl := server.MakeTmpl(StaticFs, "SpecScans")
//     tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
//     content := server.TmplPage(StaticFs, "specscans.tmpl", tmpl)
//     c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
// }

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

// PublishSrvHandler provides access to GET /piublish endpoint
func PublishSrvHandler(c *gin.Context) {
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

// DatasetsHandler provides access to GET /datasets endpoint
func DatasetsHandler(c *gin.Context) {
	user, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
	}
	// Parse query parameters
	idx, _ := strconv.Atoi(c.DefaultQuery("idx", "0"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	// TODO: we should pass it to here via GET HTTP request
	inputAttributes := c.DefaultQuery("attrs", "")
	var attrs []string
	if len(inputAttributes) != 0 {
		for _, a := range strings.Split(inputAttributes, ",") {
			attrs = append(attrs, a)
		}
	} else {
		attrs = []string{"beamline", "btr", "cycle", "sample_name", "user"}
	}
	searchFilter := c.Query("search")
	query := "{}"
	var sortKeys []string
	skey := c.Query("sortKey")
	if skey != "" {
		sortKeys = []string{skey}
	}
	var sortOrder int
	sorder := c.Query("sortDirection")
	if sorder == "asc" {
		sortOrder = 1
	} else if sorder == "desc" {
		sortOrder = -1
	}
	if Verbose > 0 {
		log.Printf("### user=%s query=%v filter=%s, attrs=%v, idx=%d, limit=%d, skey=%s, sorder=%v", user, query, searchFilter, attrs, idx, limit, skey, sorder)
	}

	spec := makeSpec(searchFilter, attrs)
	if data, err := json.Marshal(spec); err == nil {
		query = string(data)
	}

	// obtain total number of records from BE DB for our request
	rec := services.ServiceRequest{
		Client:       "frontend",
		ServiceQuery: services.ServiceQuery{Query: query},
	}
	total, err := numberOfRecords(rec)
	if err != nil {
		log.Printf("ERROR: unable to get total number of records for %s, error %v", query, err)
		c.JSON(http.StatusBadRequest, gin.H{})
	}

	// Determine the slice based on idx and limit
	start := idx
	end := idx + limit
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	rec = services.ServiceRequest{
		Client: "frontend",
		ServiceQuery: services.ServiceQuery{
			Query:     query,
			Idx:       idx,
			Limit:     limit,
			SortKeys:  sortKeys,
			SortOrder: sortOrder},
	}
	// request only user's specific data (check user attributes)
	if user != "test" && srvConfig.Config.Frontend.CheckBtrs && srvConfig.Config.Embed.DocDb == "" {
		attrs, err := chessAttributes(user)
		if err == nil {
			// in filters use-case we update spec with filters
			spec = updateSpec(spec, attrs, "filter")
			if data, err := json.Marshal(spec); err == nil {
				query = string(data)
			}
			rec = services.ServiceRequest{
				Client: "frontend",
				ServiceQuery: services.ServiceQuery{
					Query:     query,
					Spec:      spec,
					Idx:       idx,
					Limit:     limit,
					SortKeys:  sortKeys,
					SortOrder: sortOrder},
			}
		}
	}
	resp, err := chunkOfRecords(rec)
	if resp.HttpCode != http.StatusOK {
		log.Printf("ERROR: failed request to discovery service, query %+v, response %+v", rec, resp)
		c.JSON(http.StatusBadRequest, gin.H{})
	}
	if err != nil {
		log.Printf("ERROR: failed to get chunk of data, query %+v, error %v", rec, err)
		c.JSON(http.StatusBadRequest, gin.H{})
	}

	// filter outgoing records based on our attributes
	var records []map[string]any
	for _, rec := range resp.Results.Records {
		frec := make(map[string]any)
		for _, attr := range attrs {
			frec[attr] = rec[attr]
		}
		records = append(records, frec)
	}

	// Send JSON response
	c.JSON(http.StatusOK, gin.H{
		"total":    total,
		"records":  records,
		"columns":  attrs,
		"pageSize": limit,
	})

}

// helper function to get attributes based on user's affiliation
func userAttrs(user string) []string {
	var attrs []string
	for _, obj := range _smgr.Map {
		for key, _ := range obj.Schema.Map {
			attrs = append(attrs, key)
		}

	}
	return utils.List2Set[string](attrs)
}

// DatasetsTableHandler provides access to GET /dstable endpoint
func DatasetsTableHandler(c *gin.Context) {
	user, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
	}
	tmpl := server.MakeTmpl(StaticFs, "CHESS datasets")
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	attrs := userAttrs(user)
	tmpl["Columns"] = attrs
	tmpl["DataAttributes"] = strings.Join(attrs, ",")
	tmpl["User"] = user
	if user != "test" {
		if attrs, err := chessAttributes(user); err == nil {
			tmpl["Btrs"] = attrs.Btrs
		}
	}
	content := server.TmplPage(StaticFs, "dyn_dstable.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// POST handlers
// PublishHandler handles publish request for did
func PublishHandler(c *gin.Context) {
	user, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
	}

	// defaults
	r := c.Request
	w := c.Writer
	tmpl := server.MakeTmpl(StaticFs, "Login")
	template := "success.tmpl"
	httpCode := http.StatusOK
	srvCode := services.OK

	// parse input form data
	did := r.FormValue("did")
	provider := r.FormValue("provider")
	description := r.FormValue("description")
	schema := r.FormValue("schema")
	draft := r.FormValue("draft")
	metadata := r.FormValue("metadata")
	doiPublic := false
	if draft == "" {
		doiPublic = true
	}
	writeMeta := false
	if metadata != "" {
		writeMeta = true
	}

	// publish our dataset
	doi, doiLink, err := publishDataset(user, provider, did, description, doiPublic, writeMeta)
	if Verbose > 0 {
		log.Printf("### publish did=%s provider=%s doi=%s doiLink=%s error=%v", did, provider, doi, doiLink, err)
	}
	content := fmt.Sprintf("SUCCESS:<br/>did=%s<br/>is published with<br/>doi=%s URL=%s<br/>Please note: it will take some time for DOI record to appear", did, doi, doiLink)
	if err != nil {
		template = "error.tmpl"
		httpCode = http.StatusBadRequest
		content = fmt.Sprintf("ERROR:<br/>fail to publish<br/>did=%s<br/>error=%v", did, err)
	} else {
		// update metadata with DOI information
		err = updateMetaDataDOI(user, did, schema, doi, doiLink, doiPublic)
		if err != nil {
			template = "error.tmpl"
			httpCode = http.StatusBadRequest
			content = fmt.Sprintf("ERROR:<br/>fail to update MetaData DOI for<br/>did=%s<br/>error=%v", did, err)
		} else {
			// update DOI service
			if Verbose > 0 {
				log.Println("### updateDOIService", user, did, doi)
			}
			err = updateDOIService(user, did, doi, description, writeMeta)
			if err != nil {
				template = "error.tmpl"
				httpCode = http.StatusBadRequest
				content = fmt.Sprintf("ERROR:<br/>fail to update DOIService for<br/>did=%s<br/>error=%v", did, err)
			}
		}
	}
	rec := services.Response("FrontendService", httpCode, srvCode, err)
	if r.Header.Get("Accept") == "application/json" {
		if err != nil {
			c.JSON(http.StatusBadRequest, rec)
		} else {
			c.JSON(http.StatusOK, rec)
		}
		return
	} else {
		tmpl["Content"] = content
		page := server.TmplPage(StaticFs, template, tmpl)
		w.Write([]byte(header() + page + footer()))
	}
}

// PublishFormHandler handles publish request for did
func PublishFormHandler(c *gin.Context) {

	user, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
	}

	r := c.Request
	w := c.Writer
	// get beamline value from the form
	did := r.FormValue("did")
	schema := r.FormValue("schema")
	tmpl := server.MakeTmpl(StaticFs, "Login")
	base := srvConfig.Config.Frontend.WebServer.Base
	tmpl["Base"] = base
	tmpl["Did"] = did
	tmpl["User"] = user
	tmpl["Schema"] = schema
	page := server.TmplPage(StaticFs, "publishform.tmpl", tmpl)
	w.Write([]byte(header() + page + footer()))
}

// UploadJsonHandler handles upload of JSON record
func UploadJsonHandler(c *gin.Context) {

	user, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
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
	body, err := io.ReadAll(file)
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
