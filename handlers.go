package main

// handlers module
//
// Copyright (c) 2023 - Valentin Kuznetsov <vkuznet@gmail.com>
//
import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
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
	token := authz.BearerToken(c.Request)
	if token != "" {
		// if we received HTTP request with token
		claims, e := authz.TokenClaims(token, srvConfig.Config.Authz.ClientID)
		user = claims.CustomClaims.User
		if Verbose > 1 {
			log.Printf("Token=%s user=%s, error=%v", token, user, e)
			log.Println("Claims", claims)
		}
		return user, e
	}
	if srvConfig.Config.Frontend.TestMode {
		user = "TestUser"
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
	redirectTo := c.GetHeader("Referer")
	if redirectTo == "" {
		redirectTo = DEFAULT_END_POINT
	}
	if Verbose > 1 {
		for key, values := range c.Request.Header {
			for _, value := range values {
				log.Printf("Header: %s = %s\n", key, value)
			}
		}
		log.Println("redirect HTTP request to:", redirectTo)
	}

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
	c.Redirect(http.StatusFound, redirectTo)
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
	tmpl["DOIServiceUrl"] = srvConfig.Config.Services.DOIServiceURL
	content := server.TmplPage(StaticFs, "services.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// SyncHandler provides access to GET /sync endpoint
func SyncHandler(c *gin.Context) {
	user, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
	}
	tmpl := server.MakeTmpl(StaticFs, "Sync")
	base := srvConfig.Config.Frontend.WebServer.Base
	tmpl["Base"] = base
	tmpl["User"] = user

	// request only user's specific data (check user attributes)
	var btrs []string
	if user != "test" && srvConfig.Config.Frontend.CheckBtrs && srvConfig.Config.Embed.DocDb == "" {
		fuser, err := _foxdenUser.Get(user)
		btrs = fuser.Btrs
		if err == nil {
			// check user btrs and return error if user does not have any associations with Btrs
			if len(btrs) == 0 {
				msg := fmt.Sprintf("User %s does not associated with any BTRs, search access is deined", user)
				handleError(c, http.StatusBadRequest, msg, err)
				return
			}
		}
	}
	tmpl["Btrs"] = btrs
	if token, err := newToken(user, "read+write"); err == nil {
		tmpl["SourceToken"] = token
	}
	tmpl["SourceUrl"] = "https://foxden...."
	tmpl["TargetUrl"] = srvConfig.Config.Services.FrontendURL

	// create part for status dashboard
	records, err := getSyncRecords("")
	if err != nil {
		tmpl["content"] = err.Error()
		page := server.TmplPage(StaticFs, "error.tmpl", tmpl)
		msg := string(template.HTML(page))
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}
	if c.Request.Header.Get("Accept") == "application/json" {
		c.JSON(http.StatusOK, records)
		return
	}
	cols := []string{"uuid", "source_url", "target_url", "status"}
	tmpl["Columns"] = cols
	tmpl["NColumns"] = len(cols) + 1
	tmpl["Rows"] = records

	// fill out template content
	content := server.TmplPage(StaticFs, "syncform.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// SyncStatusHandler provides access to GET /sync/status/:uuid endpoint
func SyncStatusHandler(c *gin.Context) {
	_, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
	}
	status := "unknown"
	style := "success.tmpl"
	suuid := c.Param("uuid")
	// create part for status dashboard
	records, err := getSyncRecords(suuid)
	if err != nil {
		status = err.Error()
		style = "error.tmpl"
	} else if len(records) == 0 {
		status = "remove"
	} else if len(records) != 1 {
		status = fmt.Sprintf("too many records for uuid=%s", suuid)
		style = "error.tmpl"
	} else {
		record := records[0]
		if val, ok := record["status"]; ok {
			status = fmt.Sprintf("%v", val)
		}
	}
	if c.Request.Header.Get("Accept") == "application/json" {
		c.JSON(http.StatusOK, gin.H{"uuid": suuid, "status": status})
		return
	}
	// fill out template content
	tmpl := server.MakeTmpl(StaticFs, "SyncStatus")
	content := fmt.Sprintf("uuid: %s<br/>status: %v", suuid, status)
	tmpl["Content"] = content
	tmpl["Title"] = "Synchronization status"
	page := server.TmplPage(StaticFs, style, tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+page+footer()))

}

// SyncDeleteHandler provides access to DELETE /sync/delete/:uuid endpoint
func SyncDeleteHandler(c *gin.Context) {
	_, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
	}
	suuid := c.Param("uuid")
	err = deleteSyncRecord(suuid)
	if err != nil {
		log.Println("ERROR: unable to delete sync record", suuid, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err})
		return
	}
	c.JSON(http.StatusOK, gin.H{"uuid": suuid, "status": "deleted"})
}

// SchemasHandler provides access to GET /schemas end-point
func SchemasHandler(c *gin.Context) {
	var records []map[string][]beamlines.SchemaRecord
	for _, fname := range srvConfig.Config.CHESSMetaData.SchemaFiles {
		fileName := filepath.Base(fname)
		schemaName := strings.ReplaceAll(fileName, ".json", "")
		if schema, err := _smgr.Load(fname); err == nil {
			rec := make(map[string][]beamlines.SchemaRecord)
			var schemaRecords []beamlines.SchemaRecord
			for _, r := range schema.Map {
				schemaRecords = append(schemaRecords, r)
			}
			rec[schemaName] = schemaRecords
			records = append(records, rec)
		} else {
			log.Printf("ERROR: unable to read schema file %s, error=%v", fname, err)
		}
	}
	c.JSON(http.StatusOK, records)
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

// PostProvenanceHandler provides access to GET /provenance endpoint
func PostProvenanceHandler(c *gin.Context) {
	user, err := getUser(c)
	if Verbose > 1 {
		log.Printf("PostProvenanceHandler %s user=%s error=%v", c.Request.Method, user, err)
	}
	if err != nil {
		LoginHandler(c)
		return
	}
	// read HTTP POST payload for this API
	var record map[string]any
	if err := c.ShouldBindJSON(&record); err != nil {
		msg := "unable to load HTTP payload"
		log.Println("ERROR: unable to load HTTP record payload", err)
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}

	// prepare http writer
	_httpWriteRequest.GetToken()

	// insert provenance record
	rurl := fmt.Sprintf("%s/dataset", srvConfig.Config.Services.DataBookkeepingURL)
	data, err := json.Marshal(record)
	if err != nil {
		msg := "unable to marshal input record"
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}
	if Verbose > 0 {
		log.Println("INFO: submit provenance record", rurl, string(data))
	}
	resp, err := _httpWriteRequest.Post(rurl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		msg := "unable to submit record to FOXDEN metadata service"
		log.Println("ERROR:", msg, err)
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}
	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		if data, err = io.ReadAll(resp.Body); err == nil {
			log.Printf("WARNING: unable to successfully submit provenance record, response=%+v, payload data=%v", resp, string(data))
		} else {
			log.Printf("WARNING: unable to successfully submit provenance record, response=%+v", resp)
		}
		c.JSON(resp.StatusCode, nil)
		return
	}
	if Verbose > 0 {
		log.Printf("INFO: response=%s", resp.Status)
	}
	c.JSON(http.StatusOK, nil)
}

// ParentsHandler provides access to GET /parents endpoint
func ParentsHandler(c *gin.Context) {
	user, err := getUser(c)
	if Verbose > 1 {
		log.Printf("ProvenanceHandler %s user=%s error=%v", c.Request.Method, user, err)
	}
	if err != nil {
		LoginHandler(c)
		return
	}
	r := c.Request
	did := r.FormValue("did") // extract did from post form or from /provenance?did=did

	// obtain valid token
	_httpReadRequest.GetToken()

	// get files from provenance service
	records, err := getData("parents", did)
	if err != nil {
		msg := fmt.Sprintf("unable to find parents for did=%s", did)
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}
	c.JSON(http.StatusOK, records)
}

// ProvenanceHandler provides access to GET /provenance endpoint
func ProvenanceHandler(c *gin.Context) {
	user, err := getUser(c)
	if Verbose > 1 {
		log.Printf("ProvenanceHandler %s user=%s error=%v", c.Request.Method, user, err)
	}
	if err != nil {
		LoginHandler(c)
		return
	}
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
		tmpl["Parents"] = strings.Join(makeProvenanceLinks(parents), "<br/>")
	} else {
		tmpl["Parents"] = "Not available"
	}
	if len(children) > 0 {
		tmpl["Children"] = strings.Join(makeProvenanceLinks(children), "<br/>")
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
	provRecord := provenance[0]
	// fill out necessary aux info
	for _, key := range []string{"osinfo", "environments", "scripts", "packages"} {
		/*
			// no need to look-up individual pieces since we have provenance record
			records, err = getData(key, did)
			if err != nil {
				content := errorTmpl(c, fmt.Sprintf("unable to get %s data from provenance service, error", key), err)
				c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
				return
			}
		*/
		records, _ := provRecord[key]
		if data, err := json.MarshalIndent(records, "", "  "); err == nil {
			tmpl[key] = string(data)
		}
	}

	// return JSON if requested
	if c.Request.Header.Get("Accept") == "application/json" {
		c.JSON(http.StatusOK, provenance)
		return
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
		attr := c.Query("attr")

		// Prepare redirection URL
		targetURL := fmt.Sprintf("%s/data?did=%s", srvConfig.Config.DataManagementURL, did)
		if attr != "" {
			targetURL = fmt.Sprintf("%s&attr=%s", targetURL, url.QueryEscape(attr))
		}
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

// DataHubHandler provides access to GET /datahub end-point
func DataHubHandler(c *gin.Context) {
	// check if user cookie is set, this is necessary as we do not
	// use authorization handler for / end-point
	user, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
	}
	c.Set("user", user)

	did := c.Query("did")
	if did == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing 'did' parameter"})
		return
	}
	sum := md5.Sum([]byte(did))
	didhash := hex.EncodeToString(sum[:])

	// Prepare redirection URL
	targetURL := fmt.Sprintf("%s/datahub/%s", srvConfig.Config.DataHubURL, didhash)

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
	req.Header.Set("X-Custom-Header", "DataHubRequest")

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
}

// ToolsHandler provides access to GET /tools endpoint
func ToolsHandler(c *gin.Context) {
	tmpl := server.MakeTmpl(StaticFs, "Tools")
	base := srvConfig.Config.Frontend.WebServer.Base
	tmpl["Base"] = base
	content := server.TmplPage(StaticFs, "tools.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// DidsHandler provides access to GET /dids endpoint
func DidsHandler(c *gin.Context) {
	user, err := getUser(c)
	if Verbose > 1 {
		log.Printf("DidsHandler %s user=%s error=%v", c.Request.Method, user, err)
	}
	if err != nil {
		LoginHandler(c)
		return
	}
	// get all dids from Metadata service
	_httpReadRequest.GetToken()
	rurl := fmt.Sprintf("%s/records?projection=did", srvConfig.Config.Services.MetaDataURL)
	resp, err := _httpReadRequest.Get(rurl)
	if err != nil {
		msg := "unable to get meta-data from upstream server"
		handleError(c, http.StatusInternalServerError, msg, err)
		return
	}
	// parse data records from meta-data service
	var records []map[string]string
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		content := errorTmpl(c, "unable to read response body, error", err)
		handleError(c, http.StatusBadRequest, content, err)
		return
	}

	err = json.Unmarshal(data, &records)
	if err != nil {
		content := errorTmpl(c, "unable to unmarshal response, error", err)
		handleError(c, http.StatusBadRequest, content, err)
		return
	}
	if Verbose > 1 {
		log.Printf("metadata response\n%+v", records)
	}
	// return respose JSON if requested
	if c.Request.Header.Get("Accept") == "application/json" {
		c.JSON(http.StatusOK, records)
		return
	}
	var dids []string
	for _, rec := range records {
		for _, did := range rec {
			dids = append(dids, did)
		}
	}
	page := fmt.Sprintf("<pre>%s</pre>", strings.Join(dids, "\n"))
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+page+footer()))
}

// PostRecordHandler provides access to POST /record endpoint
func PostRecordHandler(c *gin.Context) {
	user, err := getUser(c)
	if Verbose > 1 {
		log.Printf("PostRecordHandler %s user=%s error=%v", c.Request.Method, user, err)
	}
	if err != nil {
		LoginHandler(c)
		return
	}
	// read HTTP POST payload for this API
	var record map[string]any
	if err := c.ShouldBindJSON(&record); err != nil {
		msg := "unable to load HTTP payload"
		log.Println("ERROR: unable to load HTTP record payload", err)
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}

	// prepare http writer
	_httpWriteRequest.GetToken()

	// insert provenance record
	rurl := fmt.Sprintf("%s", srvConfig.Config.Services.MetaDataURL)
	data, err := json.Marshal(record)
	if err != nil {
		msg := "unable to marshal input record"
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}
	if Verbose > 0 {
		log.Println("INFO: submit record", rurl, string(data))
	}
	resp, err := _httpWriteRequest.Post(rurl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		msg := "unable to submit record to FOXDEN metadata service"
		log.Println("ERROR:", msg, err)
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}
	if resp.StatusCode != 200 {
		defer resp.Body.Close()
		if data, err = io.ReadAll(resp.Body); err == nil {
			log.Printf("WARNING: unable to successfully submit metadata record, response=%+v, payload data=%v", resp, string(data))
		} else {
			log.Printf("WARNING: unable to successfully submit metadata record, response=%+v", resp)
		}
		c.JSON(resp.StatusCode, nil)
		return
	}
	if Verbose > 0 {
		log.Printf("INFO: response=%s", resp.Status)
	}
	c.JSON(http.StatusOK, nil)
}

// RecordHandler provides access to GET /search endpoint
func RecordHandler(c *gin.Context) {
	r := c.Request
	user, err := getUser(c)
	if Verbose > 1 {
		log.Printf("RecordHandler %s user=%s error=%v", c.Request.Method, user, err)
	}
	if err != nil {
		LoginHandler(c)
		return
	}
	did := r.FormValue("did") // extract did from post form or from /provenance?did=did
	spec := make(map[string]any)
	spec["did"] = did
	rec := services.ServiceRequest{
		Client:       "frontend",
		ServiceQuery: services.ServiceQuery{Spec: spec, Idx: 0, Limit: 1},
	}
	// request only user's specific data (check user attributes)
	var btrs []string
	if user != "test" && srvConfig.Config.Frontend.CheckBtrs && srvConfig.Config.Embed.DocDb == "" {
		fuser, err := _foxdenUser.Get(user)
		btrs = fuser.Btrs
		if err == nil {
			// check user btrs and return error if user does not have any associations with Btrs
			if len(btrs) == 0 {
				msg := fmt.Sprintf("User %s does not associated with any BTRs, search access is deined", user)
				handleError(c, http.StatusBadRequest, msg, err)
				return
			}
			// in search we only update spec with user's btrs
			spec = updateSpec(spec, fuser, "search")
			rec = services.ServiceRequest{
				Client:       "frontend",
				ServiceQuery: services.ServiceQuery{Spec: spec},
			}
		}
	}
	// based on user query process request from all FOXDEN services
	idx := 0
	limit := 1
	processResults(c, rec, user, idx, limit, btrs)
}

// AdvancedSearchHandler provides access to GET /search endpoint
func AdvancedSearchHandler(c *gin.Context) {
	user, err := getUser(c)
	if Verbose > 1 {
		log.Printf("AdvancedSearchHandler %s user=%s error=%v", c.Request.Method, user, err)
	}
	if err != nil {
		LoginHandler(c)
		return
	}
	// create map of schema names vs its keys
	smap := make(map[string][]string)
	for _, fname := range srvConfig.Config.CHESSMetaData.SchemaFiles {
		fileName := filepath.Base(fname)
		schemaName := strings.ReplaceAll(fileName, ".json", "")
		if schema, err := _smgr.Load(fname); err == nil {
			var keys []string
			for _, r := range schema.Map {
				keys = append(keys, r.Key)
			}
			sort.Strings(keys)
			smap[schemaName] = keys
		} else {
			log.Printf("ERROR: unable to read schema file %s, error=%v", fname, err)
		}
	}

	// create search template form
	tmpl := server.MakeTmpl(StaticFs, "AdvancedSearch")
	tmpl["User"] = user
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	tmpl["Schemas"] = smap
	b, _ := json.Marshal(smap)
	tmpl["SchemasJSON"] = template.JS(b)
	page := server.TmplPage(StaticFs, "advanced_searchform.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+page+footer()))
}

// SearchHandler provides access to GET /search endpoint
func SearchHandler(c *gin.Context) {
	r := c.Request
	user, err := getUser(c)
	if Verbose > 1 {
		log.Printf("SearchHandler %s user=%s error=%v", c.Request.Method, user, err)
	}
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

	// add AIChat into search page if it is presented within FOXDEN configuration
	if srvConfig.Config.AIChat.Model != "" {
		tmpl["AIChat"] = server.TmplPage(StaticFs, "ai_chat.tmpl", tmpl)
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
		query = "{}"
		if Verbose > 1 {
			log.Printf("WARNING: user %s used empty query, substitue to {}\n", user)
		}
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
	var btrs []string
	if user != "test" && srvConfig.Config.Frontend.CheckBtrs && srvConfig.Config.Embed.DocDb == "" {
		fuser, err := _foxdenUser.Get(user)
		btrs = fuser.Btrs
		if err == nil {
			var spec map[string]any
			err := json.Unmarshal([]byte(query), &spec)
			if err != nil {
				msg := fmt.Sprintf("malformed query %+v, unable to create spec", query)
				handleError(c, http.StatusBadRequest, msg, err)
				return
			}
			// check user btrs and return error if user does not have any associations with Btrs
			if len(btrs) == 0 {
				msg := fmt.Sprintf("User %s does not associated with any BTRs, search access is deined", user)
				handleError(c, http.StatusBadRequest, msg, err)
				return
			}
			// in search we only update spec with user's btrs
			spec = updateSpec(spec, fuser, "search")
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
	processResults(c, rec, user, idx, limit, btrs)
}

// SpecScansTableHandler provides access to GET /specscans endpoint
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

	var response services.ServiceResponse
	err = json.Unmarshal(data, &response)
	if err != nil {
		rec := services.Response("Frontend", http.StatusBadRequest, services.BindError, err)
		c.JSON(http.StatusBadRequest, rec)
		return
	}
	if Verbose > 0 {
		log.Printf("response: %+v\n", response)
	}
	scans := response.Results.Records
	if Verbose > 0 {
		log.Printf("scans: %+v\n", scans)
	}
	// Get column headers from matching scan records
	colsSet := make(map[string]struct{})
	for _, s := range scans {
		if Verbose > 2 {
			log.Printf("s: %+v\n", s)
		}
		for k, _ := range s {
			if Verbose > 2 {
				log.Printf("k: %+v\n", k)
			}
			colsSet[k] = struct{}{}
		}
	}
	cols := make([]string, 0, len(colsSet))
	for k := range colsSet {
		cols = append(cols, k)
	}
	if Verbose > 1 {
		log.Printf("colsSet: %+v", colsSet)
		log.Printf("cols: %+v", cols)
	}

	// Make table
	tmpl := server.MakeTmpl(StaticFs, "scantable.tmpl")
	tmpl["Title"] = fmt.Sprintf("Scans for DID: %s", did)
	tmpl["Columns"] = cols
	tmpl["Selected"] = map[string]bool{
		"start_time":  true,
		"spec_file":   true,
		"scan_number": true,
		"command":     true,
	}
	tmpl["Rows"] = scans
	if Verbose > 2 {
		log.Printf("tmpl: %+v\n", tmpl)
	}
	content := server.TmplPage(StaticFs, "scantable.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
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
func parseFormUploadForm(c *gin.Context) (services.MetaRecord, bool, error) {
	var updateMetadata bool
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
		return mrec, updateMetadata, err
	}
	desc := ""
	// r.PostForm provides url.Values which is map[string][]string type
	// we convert it to Record
	err = r.ParseMultipartForm(10 << 20) // 10 MB max memory
	if err != nil {
		log.Println("ERROR", err)
		return mrec, updateMetadata, err
	}
	/*
		log.Println("######## PostForm", r.PostForm)
		for key, vals := range r.MultipartForm.Value {
			log.Printf("Form field: %s = %v", key, vals)
		}
		for key, files := range r.MultipartForm.File {
			log.Printf("File field: %s = %v", key, files)
		}
	*/

	rec := make(map[string]any)
	userMetadata := make(map[string]any)
	var userKeys, userValues []string
	for k, vals := range r.PostForm {
		items := utils.UniqueFormValues(vals)
		if Verbose > 0 {
			log.Printf("### PostForm key=%s items=%v type(items)=%T", k, items, items)
		}
		if k == "user_keys" {
			for _, k := range vals {
				userKeys = append(userKeys, k)
			}
			continue
		}
		if k == "user_values" {
			for _, k := range vals {
				userValues = append(userValues, k)
			}
			continue
		}
		if k == "SchemaName" || k == "User" || k == "user_metadata" {
			continue
		}
		if k == "Description" {
			desc = strings.Join(items, " ")
			continue
		}
		if k == "UpdateMetadata" {
			for _, k := range vals {
				log.Println("### checkbox UpdateMetadata", k)
				if k == "on" {
					updateMetadata = true
				}
			}
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
					return mrec, updateMetadata, err
				}
			} else {
				if !utils.InList(k, beamlines.SkipKeys) {
					log.Printf("ERROR: no key=%s found in schema=%+v, error %v", k, schema, err)
					return mrec, updateMetadata, err
				}
			}
		}
		rec[k] = val
	}

	// parse user metafile if it is provided
	files := r.MultipartForm.File["user_metadata"]
	if len(files) == 1 {
		uploadedFile := files[0]
		file, err := uploadedFile.Open()
		if err != nil {
			log.Printf("ERROR: unable to load metadata file error %v", err)
		}
		defer file.Close()
		body, err := io.ReadAll(file)
		if err == nil {
			// try to load it as JSON
			var record map[string]any
			if e := json.Unmarshal(body, &record); e == nil {
				userMetadata["metadata"] = record
			} else {
				userMetadata["metadata"] = fmt.Sprintf("%v", string(body))
			}
		} else {
			log.Printf("ERROR: unable to load metadata %v, error %v", fname, err)
		}
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
	if len(userKeys) != 0 && len(userValues) != 0 && len(userKeys) == len(userValues) {
		for i := 0; i < len(userKeys); i++ {
			if userKeys[i] != "" && userValues[i] != "" {
				userMetadata[userKeys[i]] = userValues[i]
			}
		}
		rec["user_metadata"] = userMetadata
	}
	if Verbose > 0 {
		log.Printf("process form, record %v\n", rec)
	}
	mrec.Record = rec
	return mrec, updateMetadata, nil
}

// MetaFormUploadHandler provides access to GET /meta/form/upload endpoint
func MetaFormUploadHandler(c *gin.Context) {
	rec, updateMetadata, err := parseFormUploadForm(c)
	if err != nil {
		handleError(c, http.StatusBadRequest, "unable to parse file upload form", err)
		return
	}
	if rec.Schema == "user" {
		UserUploadHandler(c, rec, updateMetadata)
	} else {
		MetaUploadHandler(c, rec, updateMetadata)
	}
}

// MetaFileUploadHandler provides access to GET /meta/file/upload endpoint
func MetaFileUploadHandler(c *gin.Context) {
	rec, err := parseFileUploadForm(c)
	if err != nil {
		handleError(c, http.StatusBadRequest, "unable to parse file upload form", err)
		return
	}
	if rec.Schema == "user" {
		UserUploadHandler(c, rec, false)
	} else {
		MetaUploadHandler(c, rec, false)
	}
}

// UserUploadHandler manages upload of user record to Metadata service
func UserUploadHandler(c *gin.Context, mrec services.MetaRecord, updateMetadata bool) {
	class := "alert alert-success"
	user, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
	}
	tmpl := server.MakeTmpl(StaticFs, "Upload")
	mrec.Record["user"] = user
	if Verbose > 0 {
		log.Printf("user record %+v", mrec)
	}
	var did string
	if val, ok := mrec.Record["parent_did"]; ok {
		tstamp := time.Now().Format("20060102_150405")
		did = fmt.Sprintf("%s/user=%s:%s", val, user, tstamp)
		mrec.Record["did"] = did
	}
	mrec.Record["beamline"], mrec.Record["btr"], mrec.Record["cycle"], mrec.Record["sample_name"] = extractParts(did)
	if mrec.Record["btr"] == "" {
		class = "alert alert-error"
		msg := "unable to extract btr from did of the record"
		log.Printf("ERROR: %s %+v", msg, mrec)
		content := errorTmpl(c, msg, nil)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}
	mrec.Record["input_files"] = formFiles(mrec.Record["input_files"])
	mrec.Record["output_files"] = formFiles(mrec.Record["output_files"])

	// fill out required provenance info in services.MetaRecord
	provRecord := provenanceRecord(mrec)

	// prepare http writer
	_httpWriteRequest.GetToken()

	// insert provenance record
	rurl := fmt.Sprintf("%s/provenance", srvConfig.Config.Services.DataBookkeepingURL)
	data, err := json.MarshalIndent(provRecord, "", "  ")
	if err != nil {
		class = "alert alert-error"
		content := errorTmpl(c, "unable to marshal provenance record, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}
	var resp *http.Response
	if updateMetadata {
		resp, err = _httpWriteRequest.Put(rurl, "application/json", bytes.NewBuffer(data))
	} else {
		resp, err = _httpWriteRequest.Post(rurl, "application/json", bytes.NewBuffer(data))
	}
	if err != nil {
		class = "alert alert-error"
		content := errorTmpl(c, "unable to insert provenance record, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}

	// place request to MetaData service
	rurl = fmt.Sprintf("%s", srvConfig.Config.Services.MetaDataURL)
	data, err = json.MarshalIndent(mrec, "", "  ")
	if err != nil {
		class = "alert alert-error"
		content := errorTmpl(c, "unable to marshal meta data record, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}
	resp, err = _httpWriteRequest.Post(rurl, "application/json", bytes.NewBuffer(data))
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
		class = "alert alert-error"
		msg = fmt.Sprintf("<pre>%s<pre>", sresp.String())
	}

	// we should use metadata json record instead of services.MetaRecord for web form
	if data, err := json.MarshalIndent(mrec.Record, "", "  "); err == nil {
		tmpl["JsonRecord"] = template.HTML(string(data))
	} else {
		erec := make(map[string]any)
		erec["error"] = err
		tmpl["JsonRecord"] = erec
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

// MetaUploadHandler manages upload of record to MetaData service
func MetaUploadHandler(c *gin.Context, mrec services.MetaRecord, updateMetadata bool) {
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
	var resp *http.Response
	if updateMetadata {
		resp, err = _httpWriteRequest.Put(rurl, "application/json", bytes.NewBuffer(data))
	} else {
		resp, err = _httpWriteRequest.Post(rurl, "application/json", bytes.NewBuffer(data))
	}
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

	// we should use metadata json record instead of services.MetaRecord for web form
	if data, err := json.MarshalIndent(mrec.Record, "", "  "); err == nil {
		tmpl["JsonRecord"] = template.HTML(string(data))
	} else {
		erec := make(map[string]any)
		erec["error"] = err
		tmpl["JsonRecord"] = erec
	}
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	tmpl["User"] = user
	tmpl["Date"] = time.Now().Unix()
	tmpl["Schema"] = mrec.Schema
	tmpl["Message"] = msg
	tmpl["Status"] = sresp.Status
	tmpl["Class"] = class
	if sresp.Status == "error" || sresp.SrvCode != 0 {
		tmpl["Class"] = "alert alert-error"
	}
	tmpl["ResponseRecord"] = template.HTML(sresp.JsonString())
	content := server.TmplPage(StaticFs, "upload_status.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// ProvInfoHandler provides access to GET /info/provenance endpoint
func ProvInfoHandler(c *gin.Context) {
	tmpl := server.MakeTmpl(StaticFs, "Provenance information")
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	content := server.TmplPage(StaticFs, "provinfo.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// SpecScansInfoHandler provides access to GET /info/scanspecs endpoint
func SpecScansInfoHandler(c *gin.Context) {
	tmpl := server.MakeTmpl(StaticFs, "SpecScans information")
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	content := server.TmplPage(StaticFs, "specscansinfo.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// DataManagementInfoHandler provides access to GET /info/datamanagement endpoint
func DataManagementInfoHandler(c *gin.Context) {
	tmpl := server.MakeTmpl(StaticFs, "DataManagement information")
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	content := server.TmplPage(StaticFs, "datamgtinfo.tmpl", tmpl)
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
	if Verbose > 1 {
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
		fuser, err := _foxdenUser.Get(user)
		if err == nil {
			// in filters use-case we update spec with filters
			spec = updateSpec(spec, fuser, "filter")
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
		if fuser, err := _foxdenUser.Get(user); err == nil {
			tmpl["Btrs"] = fuser.Btrs
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

	// parse input form
	var parents []string
	if err := r.ParseForm(); err == nil {
		parents = r.Form["parents"]
	}
	// parse input form data
	did := r.FormValue("did")
	doiprovider := r.FormValue("doiprovider")
	description := r.FormValue("description")
	schema := r.FormValue("schema")
	draft := r.FormValue("draft")
	publishmetadata := r.FormValue("publishmetadata")
	// extract MC project name from the form
	mcprojectname := r.FormValue("mcprojectname")
	// but if foxden.yaml provides it we will overwrite it
	if srvConfig.Config.DOI.ProjectName != "" {
		mcprojectname = srvConfig.Config.DOI.ProjectName
	}
	doiPublic := false
	if draft == "" {
		doiPublic = true
	}

	// publish our dataset
	doi, doiLink, err := publishDataset(user, doiprovider, did, description, parents, doiPublic, mcprojectname)
	if Verbose > 0 {
		log.Printf("### publish did=%s doiprovider=%s doi=%s doiLink=%s error=%v", did, doiprovider, doi, doiLink, err)
	}
	content := fmt.Sprintf("SUCCESS:<br/><b>did=%s</b><br/>is published with<br/><b>DOI=%s</b><br/><b>URL=<a href=\"%s\">%s</a></b><br/>Please note: it will take some time for DOI record to appear", did, doi, doiLink, doiLink)
	if err != nil {
		template = "error.tmpl"
		httpCode = http.StatusBadRequest
		content = fmt.Sprintf("ERROR:<br/>fail to publish<br/>did=%s<br/>error=%v", did, err)
	} else if doi == "" || doiLink == "" {
		template = "error.tmpl"
		httpCode = http.StatusBadRequest
		content = fmt.Sprintf("ERROR:<br/>unable to get DOI info for <br/>did=%s<br/> from %s DOI provider", did, doiprovider)
	} else {
		// update metadata with DOI information
		err = updateMetaDataDOI(user, did, schema, doiprovider, doi, doiLink, doiPublic, publishmetadata, parents)
		if err != nil {
			template = "error.tmpl"
			httpCode = http.StatusBadRequest
			content = fmt.Sprintf("ERROR:<br/>fail to update MetaData DOI for<br/>did=%s<br/>error=%v", did, err)
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
	base := srvConfig.Config.Frontend.WebServer.Base
	// get beamline value from the form
	did := r.FormValue("did")
	schema := r.FormValue("schema")
	tmpl := server.MakeTmpl(StaticFs, "Login")
	tmpl["Base"] = base
	tmpl["Did"] = did
	tmpl["User"] = user
	tmpl["Schema"] = schema
	tmpl["MCProjectName"] = r.FormValue("mcprojectname")
	tmpl["Parents"] = getAllParents(did)
	page := server.TmplPage(StaticFs, "publishform.tmpl", tmpl)
	w.Write([]byte(header() + page + footer()))
}

// DoiPublicHandler handles publishing given DOI as public record
func DoiPublicHandler(c *gin.Context) {
	user, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
	}
	r := c.Request
	w := c.Writer
	doi := r.FormValue("doi")
	did := r.FormValue("did")
	doiLink := r.FormValue("doilink")
	schema := r.FormValue("schema")
	doiprovider := r.FormValue("doiprovider")
	tmpl := server.MakeTmpl(StaticFs, "Login")
	template := "success.tmpl"
	content := fmt.Sprintf("SUCCESS:<br/><b>DOI=%s</b><br/>is published with %s as public DOI<br/><b>URL=<a href=\"%s\">%s</a></b><br/>Please note: it will take some time for public DOI record to appear", doi, doiprovider, doiLink, doiLink)

	// update dataset info in DOI provider
	if err := makePublic(doi, doiprovider); err == nil {
		// update DOI info in MetaData service to make it public
		doiPublic := true
		doiParents := []string{}
		if err := updateMetaDataDOI(user, did, schema, doiprovider, doi, doiLink, doiPublic, "preserve", doiParents); err != nil {
			template = "error.tmpl"
			content = fmt.Sprintf("ERROR:<br/>fail to update Metadata DOI information<br/>DOI=%s<br/>error=%v", doi, err)
		}
	} else {
		template = "error.tmpl"
		content = fmt.Sprintf("ERROR:<br/>fail to create public DOI record<br/>DOI=%s<br/>error=%v", doi, err)
	}
	tmpl["Content"] = content
	page := server.TmplPage(StaticFs, template, tmpl)
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

/* DEVELOPMENT: AIChatHandler for assisting AI chat requests */

// ChatRequest represents incoming JSON from frontend
type ChatRequest struct {
	Message string `json:"message"`
}

// ChatResponse represents outgoing JSON to frontend
type ChatResponse struct {
	Reply string `json:"reply"`
}

// AIChatHandler handles requests from AI assitance chat
func AIChatHandler(c *gin.Context) {
	user, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
	}

	var req ChatRequest

	// Bind JSON body to struct
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	// send request to AI chatbot
	resp, err := aichat(user, req.Message)
	if err != nil {
		c.JSON(http.StatusBadRequest, ChatResponse{Reply: err.Error()})
	}

	c.JSON(http.StatusOK, ChatResponse{Reply: resp})
}

// AmendFormHandler provides access to GET /amend endpoint
func AmendFormHandler(c *gin.Context) {
	_, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
	}
	w := c.Writer
	tmpl := server.MakeTmpl(StaticFs, "Amend")
	base := srvConfig.Config.Frontend.WebServer.Base
	did := c.Query("did")
	// find meta-data record for provided did
	record, err := findMetadataRecord(did)
	if err != nil {
		tmpl["Content"] = fmt.Sprintf("Unable to find metadata record for did=%s, error=%v", did, err)
		page := server.TmplPage(StaticFs, "error.tmpl", tmpl)
		w.Write([]byte(header() + page + footer()))
		return
	}
	// find meta-data record with did
	tmpl["Base"] = base
	tmpl["Did"] = did
	if val, err := json.MarshalIndent(record, "", "  "); err == nil {
		tmpl["Record"] = string(val)
	} else {
		tmpl["Record"] = record
	}
	content := server.TmplPage(StaticFs, "amend.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+content+footer()))
}

// AddAuxDataHandler provides access to POST /addauxdata endpoint
func AddAuxDataHandler(c *gin.Context) {
	user, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
	}
	tmpl := server.MakeTmpl(StaticFs, "AmendForm")
	r := c.Request
	did := r.FormValue("did")

	// use user data
	file, fheader, err := r.FormFile("file")
	if err != nil {
		msg := "unable to obtain user file"
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}
	defer file.Close()
	body, err := io.ReadAll(file)

	// create temp file with user data and the same user's file name
	path := filepath.Join(os.TempDir(), fheader.Filename)
	tmpFile, err := os.Create(path)
	if err != nil {
		msg := "unable to obtain temp file"
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}
	defer os.Remove(tmpFile.Name())
	_, err = tmpFile.Write(body)
	if err != nil {
		msg := "unable to write to temp file"
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}
	if err := tmpFile.Close(); err != nil {
		msg := "unable to close temp file"
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}

	rec := make(map[string]string)
	rec["did"] = did
	rec["file"] = tmpFile.Name()

	// compose request to DataHub service
	targetURL := fmt.Sprintf("%s/datahub", srvConfig.Config.DataHubURL)

	// get new read token
	token, err := newToken(user, "write")
	if err != nil {
		msg := "failed to obtain write token"
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}
	data, err := json.Marshal(rec)
	if err != nil {
		msg := "failed to marshal request data"
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}

	// Create a new HTTP request to the target URL
	req, err := http.NewRequest(http.MethodPost, targetURL, bytes.NewBuffer(data))
	if err != nil {
		msg := "failed to place HTTP request to DataHub"
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}

	// Set custom headers
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Custom-Header", "DataHubRequest")

	// Send the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		msg := "failed to upload data to DataHub"
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}
	content := fmt.Sprintf("record with did=%s has been successfully uploaded new aux data", did)
	template := "success.tmpl"
	if resp.StatusCode != 200 {
		content = fmt.Sprintf("record with did=%s failed to upload aux data", did)
		template = "error.tmpl"
	}
	tmpl["Content"] = content
	page := server.TmplPage(StaticFs, template, tmpl)
	c.Writer.Write([]byte(header() + page + footer()))
}

// AmendRecordHandler provides access to POST /amend endpoint
func AmendRecordHandler(c *gin.Context) {
	user, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
	}
	tmpl := server.MakeTmpl(StaticFs, "AmendForm")
	r := c.Request
	w := c.Writer
	did := r.FormValue("did")
	recStr := r.FormValue("record")
	var rec map[string]any
	template := "success.tmpl"
	content := fmt.Sprintf("Record %s is successfully updated", did)
	status := http.StatusOK
	if err := json.Unmarshal([]byte(recStr), &rec); err == nil {
		if _, ok := rec["user"]; !ok {
			rec["user"] = user
		}
		// update meta-data record
		err := updateMetadataRecord(did, rec)
		if err != nil {
			content = fmt.Sprintf("Record %s update fails with error=%v", did, err)
			template = "error.tmpl"
			status = http.StatusBadRequest
		}
	} else {
		content = fmt.Sprintf("Record %s update fails with error=%v", did, err)
		template = "error.tmpl"
		status = http.StatusBadRequest
	}

	httpCode := http.StatusOK
	srvCode := services.OK
	resp := services.Response("FrontendService", httpCode, srvCode, err)
	if r.Header.Get("Accept") == "application/json" {
		if err != nil {
			c.JSON(http.StatusBadRequest, resp)
		} else {
			c.JSON(http.StatusOK, resp)
		}
		return
	} else {
		tmpl["Content"] = content
		page := server.TmplPage(StaticFs, template, tmpl)
		w.WriteHeader(status)
		w.Write([]byte(header() + page + footer()))
	}
}

// SyncFormHandler provides access to POST /sync endpoint
func SyncFormHandler(c *gin.Context) {
	user, err := getUser(c)
	if err != nil {
		LoginHandler(c)
		return
	}
	// prepare our data
	tmpl := server.MakeTmpl(StaticFs, "Sync")
	base := srvConfig.Config.Frontend.WebServer.Base
	tmpl["Base"] = base
	tmpl["User"] = user
	rec := make(map[string]any)
	rec["source_url"] = c.Request.FormValue("sourceUrl")
	rec["source_token"] = c.Request.FormValue("sourceToken")
	rec["target_url"] = c.Request.FormValue("targetUrl")
	rec["target_token"] = c.Request.FormValue("targetToken")

	// optional parameters
	rec["did"] = c.Request.FormValue("did")
	if err := c.Request.ParseForm(); err == nil {
		rec["btrs"] = c.Request.Form["btrs"]
	}

	// serialize sync record
	data, err := json.Marshal(rec)
	if err != nil {
		handleError(c, http.StatusBadRequest, "unable to process sync request form", err)
		return
	}
	// insert sync record
	_httpWriteRequest.GetToken()
	rurl := fmt.Sprintf("%s/record", srvConfig.Config.Services.SyncServiceURL)
	resp, err := _httpWriteRequest.Post(rurl, "application/json", bytes.NewBuffer(data))
	if err != nil || resp.StatusCode != 200 {
		msg := fmt.Sprintf("unable to process sync request, status %s", resp.Status)
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}
	if c.Request.Header.Get("Accept") == "application/json" {
		c.JSON(http.StatusOK, nil)
		return
	}
	content := fmt.Sprintf("Sync record successfully created, you will be redirected to sync page in few seconds...")
	tmpl["Content"] = content
	tmpl["RedirectLink"] = fmt.Sprintf("%s/sync", base)
	page := server.TmplPage(StaticFs, "success.tmpl", tmpl)
	c.Writer.Write([]byte(header() + page + footer()))
}
