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

	authz "github.com/CHESSComputing/common/authz"
	srvConfig "github.com/CHESSComputing/common/config"
	utils "github.com/CHESSComputing/common/utils"
	"github.com/dchest/captcha"
	"github.com/gin-gonic/gin"
)

// Documentation about gib handlers can be found over here:
// https://go.dev/doc/tutorial/web-service-gin

//
// Data structure we use through the code
//

// UserRegistationForm represents site registration form on web UI
type UserRegistrationForm struct {
	Login           string `form:"login" json:"login"`
	Password        string `form:"password" json:"password"`
	FirstName       string `form:"first_name" json:"first_name"`
	LastName        string `form:"last_name" json:"last_name"`
	Email           string `form:"email" json:"email"`
	CaptchaID       string `form:"captchaId" json:",omitempty"`
	CaptchaSolution string `form:"captchaSolution" json:",omitempty"`
}

// DocsParams represents URI storage params in /docs/:page end-point
type DocsParams struct {
	Page string `uri:"page" binding:"required"`
}

// LoginForm represents login form
type LoginForm struct {
	User     string `form:"user" binding:"required"`
	Password string `form:"password" binding:"required"`
}

// User represents structure used by users DB in Authz service to handle incoming requests
type User struct {
	Login    string
	Password string
}

//
// helper functions
//

// helper function to provides error template message
func errorTmpl(c *gin.Context, msg string, err error) string {
	tmpl := makeTmpl(c, "Status")
	tmpl["Content"] = template.HTML(fmt.Sprintf("<div>%s</div>\n<br/><h3>ERROR</h3>%v", msg, err))
	content := utils.TmplPage(StaticFs, "error.tmpl", tmpl)
	return content
}

// helper functiont to provides success template message
func successTmpl(c *gin.Context, msg string) string {
	tmpl := makeTmpl(c, "Status")
	tmpl["Content"] = template.HTML(fmt.Sprintf("<h3>SUCCESS</h3><div>%s</div>", msg))
	content := utils.TmplPage(StaticFs, "success.tmpl", tmpl)
	return content
}

//
// GET handlers
//

// CaptchaHandler provides access to captcha server
func CaptchaHandler() gin.HandlerFunc {
	hdlr := captcha.Server(captcha.StdWidth, captcha.StdHeight)
	return func(c *gin.Context) {
		hdlr.ServeHTTP(c.Writer, c.Request)
	}
}

// IndexHandler provides access to GET / end-point
func IndexHandler(c *gin.Context) {
	// check if user cookie is set, this is necessary as we do not
	// use authorization handler for / end-point
	user, err := c.Cookie("user")
	if err == nil {
		c.Set("user", user)
	} else {
		log.Println("WARNING: unable to get user cookie", err)
	}
	if srvConfig.Config.Frontend.WebServer.Verbose > 0 {
		log.Printf("user from c.Cookie: '%s'", user)
	}

	// top and bottom HTTP content from our templates
	tmpl := makeTmpl(c, "Home")
	top := utils.TmplPage(StaticFs, "top.tmpl", tmpl)
	bottom := utils.TmplPage(StaticFs, "bottom.tmpl", tmpl)
	tmpl["LogoClass"] = "show"
	tmpl["MapClass"] = "hide"
	if user != "" {
		tmpl["LogoClass"] = "hide"
		tmpl["MapClass"] = "show"
		tmpl["Users"] = user
	}
	content := utils.TmplPage(StaticFs, "index.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(top+content+bottom))
}

// DocsHandler provides access to GET /docs end-point
func DocsHandler(c *gin.Context) {
	// check if user cookie is set, this is necessary as we do not
	// use authorization handler for /docs end-point
	if user, err := c.Cookie("user"); err == nil {
		c.Set("user", user)
	}
	tmpl := makeTmpl(c, "Documentation")
	top := utils.TmplPage(StaticFs, "top.tmpl", tmpl)
	bottom := utils.TmplPage(StaticFs, "bottom.tmpl", tmpl)
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
	content = utils.TmplPage(StaticFs, "content.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(top+content+bottom))
}

// LoginHandler provides access to GET /login endpoint
func LoginHandler(c *gin.Context) {
	tmpl := makeTmpl(c, "Login")
	top := utils.TmplPage(StaticFs, "top.tmpl", tmpl)
	bottom := utils.TmplPage(StaticFs, "bottom.tmpl", tmpl)
	content := utils.TmplPage(StaticFs, "login.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(top+content+bottom))
}

// LogoutHandler provides access to GET /logout endpoint
func LogoutHandler(c *gin.Context) {
	c.SetCookie("user", "", -1, "/", domain(), false, true)
	c.Redirect(http.StatusFound, "/")
}

// UserRegistryHandler provides access to GET /registry endpoint
func UserRegistryHandler(c *gin.Context) {
	// check if user cookie is set, this is necessary as we do not
	// use authorization handler for /registry end-point
	if user, err := c.Cookie("user"); err == nil {
		c.Set("user", user)
	}
	tmpl := makeTmpl(c, "User registration")
	top := utils.TmplPage(StaticFs, "top.tmpl", tmpl)
	bottom := utils.TmplPage(StaticFs, "bottom.tmpl", tmpl)
	captchaStr := captcha.New()
	if srvConfig.Config.Frontend.WebServer.Verbose > 0 {
		log.Println("new captcha", captchaStr)
	}
	tmpl["CaptchaId"] = captchaStr
	tmpl["CaptchaPublicKey"] = srvConfig.Config.Frontend.CaptchaPublicKey
	content := utils.TmplPage(StaticFs, "user_registration.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(top+content+bottom))
}

// POST handlers

// LoginPostHandler provides access to POST /login endpoint
func LoginPostHandler(c *gin.Context) {
	tmpl := makeTmpl(c, "Login")
	top := utils.TmplPage(StaticFs, "top.tmpl", tmpl)
	bottom := utils.TmplPage(StaticFs, "bottom.tmpl", tmpl)
	var form LoginForm
	var content string
	var err error

	if err = c.ShouldBind(&form); err != nil {
		content = errorTmpl(c, "login form binding error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(top+content+bottom))
		return
	}

	// encrypt provided user password before sending to Authz server
	form, err = encryptLoginObject(form)
	if err != nil {
		content = errorTmpl(c, "unable to encrypt user password", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(top+content+bottom))
		return
	}

	// make a call to Authz service to check for a user
	rurl := fmt.Sprintf("%s/oauth/authorize?client_id=%s&response_type=code", srvConfig.Config.Services.AuthzURL, srvConfig.Config.Authz.ClientId)
	user := User{Login: form.User, Password: form.Password}
	data, err := json.Marshal(user)
	if err != nil {
		content = errorTmpl(c, "unable to marshal user form, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(top+content+bottom))
		return
	}
	resp, err := http.Post(rurl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		content = errorTmpl(c, "unable to POST request to Authz service, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(top+content+bottom))
		return
	}
	defer resp.Body.Close()
	data, err = io.ReadAll(resp.Body)
	var response authz.Response
	err = json.Unmarshal(data, &response)
	if err != nil {
		content = errorTmpl(c, "unable handle authz response, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(top+content+bottom))
		return
	}
	if srvConfig.Config.Frontend.WebServer.Verbose > 0 {
		log.Printf("INFO: Authz response %+v, error %v", response, err)
	}
	if response.Status != "ok" {
		msg := fmt.Sprintf("No user %s found in Authz service", form.User)
		content = errorTmpl(c, msg, errors.New("user not found"))
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(top+content+bottom))
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

// UserRegistryPostHandler provides access to POST /registry endpoint
func UserRegistryPostHandler(c *gin.Context) {
	tmpl := makeTmpl(c, "User registration")
	top := utils.TmplPage(StaticFs, "top.tmpl", tmpl)
	bottom := utils.TmplPage(StaticFs, "bottom.tmpl", tmpl)

	// parse input form request
	var form UserRegistrationForm
	var err error
	content := successTmpl(c, "User registation is completed")

	if err = c.ShouldBind(&form); err != nil {
		content = errorTmpl(c, "User registration binding error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(top+content+bottom))
		return
	}
	if srvConfig.Config.Frontend.WebServer.Verbose > 0 {
		log.Printf("new user %+v", form)
	}

	// first check if user provides the captcha
	if !captcha.VerifyString(form.CaptchaID, form.CaptchaSolution) {
		msg := "Wrong captcha match, robots are not allowed"
		content = errorTmpl(c, msg, err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(top+content+bottom))
		return
	}

	// encrypt form password
	form, err = encryptUserObject(form)
	if err != nil {
		content = errorTmpl(c, "unable to encrypt user password", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(top+content+bottom))
		return
	}

	// make a call to Authz service to registry new user
	rurl := fmt.Sprintf("%s/user", srvConfig.Config.Services.AuthzURL)
	data, err := json.Marshal(form)
	if err != nil {
		content = errorTmpl(c, "unable to marshal user form, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(top+content+bottom))
		return
	}
	resp, err := http.Post(rurl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		content = errorTmpl(c, "unable to POST request to Authz service, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(top+content+bottom))
		return
	}
	defer resp.Body.Close()
	data, err = io.ReadAll(resp.Body)
	var response authz.Response
	err = json.Unmarshal(data, &response)
	if err != nil {
		content = errorTmpl(c, "unable handle authz response, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(top+content+bottom))
		return
	}
	if srvConfig.Config.Frontend.WebServer.Verbose > 0 {
		log.Printf("INFO: Authz response %+v, error %v", response, err)
	}
	if response.Status != "ok" {
		msg := fmt.Sprintf("No user %s found in Authz service", form.Login)
		content = errorTmpl(c, msg, errors.New("user not found"))
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(top+content+bottom))
		return
	}

	c.Set("user", form.Login)
	if srvConfig.Config.Frontend.WebServer.Verbose > 0 {
		log.Printf("login from user %s, url path %s", form.Login, c.Request.URL.Path)
	}

	// set our user cookie
	if _, err := c.Cookie("user"); err != nil {
		if srvConfig.Config.Frontend.WebServer.Verbose > 0 {
			log.Printf("user registry: set cookie user=%s domain=%s", form.Login, domain())
		}
		c.SetCookie("user", form.Login, 3600, "/", domain(), false, true)
		c.Set("user", form.Login)
	}

	// return page
	// we regenerate top template with new user info
	top = utils.TmplPage(StaticFs, "top.tmpl", tmpl)
	// create page content
	tmpl["Content"] = template.HTML(content)
	content = utils.TmplPage(StaticFs, "success.tmpl", tmpl)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(top+content+bottom))
}
