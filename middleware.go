package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	authz "github.com/CHESSComputing/golib/authz"
	srvConfig "github.com/CHESSComputing/golib/config"
	"github.com/gin-gonic/gin"
	jwt "github.com/golang-jwt/jwt/v4"
	cryptoutils "github.com/vkuznet/cryptoutils"
)

// _token is used across all authorized APIs
var _token *authz.Token

// gin cookies
// https://gin-gonic.com/docs/examples/cookie/
// more advanced use-case:
// https://stackoverflow.com/questions/66289603/use-existing-session-cookie-in-gin-router
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// check if our user key is set
		if user, err := c.Cookie("user"); err == nil {
			if srvConfig.Config.Frontend.WebServer.Verbose > 0 {
				log.Println(c.Request.Method, c.Request.URL.Path, user)
			}
			c.Set("user", user)
			if err := refreshToken(); err != nil {
				c.SetCookie("user", "", -1, "/", "localhost", false, true)
				c.Set("user", "")
				content := errorTmpl(c, "unable to get valid token", err)
				log.Fatal(content)
				//                 c.Data(http.StatusUnauthorized, "text/html; charset=utf-8", []byte(content))
				//                 c.Redirect(http.StatusFound, "/")
				return
			}
			return
		}

		if user, ok := c.Get("user"); !ok {
			if srvConfig.Config.Frontend.WebServer.Verbose > 0 {
				log.Println(c.Request.Method, c.Request.URL.Path)
			}
			c.Redirect(http.StatusFound, "/login")
		} else {
			if srvConfig.Config.Frontend.WebServer.Verbose > 0 {
				log.Println(c.Request.Method, c.Request.URL.Path, user)
			}
		}
		if err := refreshToken(); err != nil {
			content := errorTmpl(c, "unable to get valid token", err)
			c.Data(http.StatusUnauthorized, "text/html; charset=utf-8", []byte(content))
			return
		}
		c.Next()
	}
}

// helper function to refresh global token used in authorized APIs
func refreshToken() error {
	// check and obtain token
	var err error
	if _token == nil {
		if token, err := getToken(); err == nil {
			_token = &token
		} else {
			return err
		}
	} else {
		err = _token.Validate(srvConfig.Config.Authz.ClientID)
	}
	return err
}

// helper function to obtain JWT token from Authz service
func getToken() (authz.Token, error) {
	var token authz.Token
	rurl := fmt.Sprintf("%s/oauth/token?client_id=%s&client_secret=%s&grant_type=client_credentials&scope=read", srvConfig.Config.Services.AuthzURL, srvConfig.Config.Authz.ClientID, srvConfig.Config.Authz.ClientSecret)
	resp, err := http.Get(rurl)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return token, err
	}
	err = json.Unmarshal(data, &token)
	if err != nil {
		return token, err
	}
	reqToken := token.AccessToken
	if srvConfig.Config.Frontend.WebServer.Verbose > 0 {
		log.Printf("INFO: obtain token %+v", token)
	}

	// validate our token
	var jwtKey = []byte(srvConfig.Config.Authz.ClientID)
	claims := &authz.Claims{}
	tkn, err := jwt.ParseWithClaims(reqToken, claims, func(token *jwt.Token) (any, error) {
		return jwtKey, nil
	})
	if err != nil {
		if err == jwt.ErrSignatureInvalid {
			return token, errors.New("invalid token signature")
		}
		return token, err
	}
	if !tkn.Valid {
		log.Println("WARNING: token invalid")
		return token, errors.New("invalid token validity")
	}
	return token, nil
}

// helper function to perform HTTP GET request with bearer token
func httpGet(rurl string) (*http.Response, error) {
	req, err := http.NewRequest("GET", rurl, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", _token.AccessToken))
	req.Header.Add("Accept-Encoding", "")
	client := &http.Client{}
	if srvConfig.Config.Frontend.WebServer.Verbose > 1 {
		dump, err := httputil.DumpRequestOut(req, true)
		log.Println("request", string(dump), err)
	}
	resp, err := client.Do(req)
	if srvConfig.Config.Frontend.WebServer.Verbose > 1 {
		dump, err := httputil.DumpResponse(resp, true)
		log.Println("response", string(dump), err)
	}
	return resp, err
}

// helper function to perform HTTP POST request with bearer token
func httpPost(rurl, contentType string, buffer *bytes.Buffer) (*http.Response, error) {
	req, err := http.NewRequest("POST", rurl, buffer)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", _token.AccessToken))
	req.Header.Add("Content-Type", contentType)
	req.Header.Add("Accept", contentType)
	client := &http.Client{}
	if srvConfig.Config.Frontend.WebServer.Verbose > 1 {
		dump, err := httputil.DumpRequestOut(req, true)
		log.Println("request", string(dump), err)
	}
	resp, err := client.Do(req)
	if srvConfig.Config.Frontend.WebServer.Verbose > 1 {
		dump, err := httputil.DumpResponse(resp, true)
		log.Println("response", string(dump), err)
	}
	return resp, err
}

// helper function to perform HTTP POST form request with bearer token
func httpPostForm(rurl string, formData url.Values) (*http.Response, error) {
	req, err := http.NewRequest("POST", rurl, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", _token.AccessToken))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{}
	if srvConfig.Config.Frontend.WebServer.Verbose > 1 {
		dump, err := httputil.DumpRequestOut(req, true)
		log.Println("request", string(dump), err)
	}
	resp, err := client.Do(req)
	if srvConfig.Config.Frontend.WebServer.Verbose > 1 {
		dump, err := httputil.DumpResponse(resp, true)
		log.Println("response", string(dump), err)
	}
	return resp, err
}

// helper function to encrypt user registration form attributes
func encryptUserObject(form UserRegistrationForm) (UserRegistrationForm, error) {
	encryptedObject, err := cryptoutils.HexEncrypt(
		form.Password, srvConfig.Config.Encryption.Secret, srvConfig.Config.Encryption.Cipher)
	if err != nil {
		return form, err
	} else {
		form.Password = encryptedObject
	}
	return form, nil
}

// helper function to encrypt login form attributes
func encryptLoginObject(form LoginForm) (LoginForm, error) {
	encryptedObject, err := cryptoutils.HexEncrypt(
		form.Password, srvConfig.Config.Encryption.Secret, srvConfig.Config.Encryption.Cipher)
	if err != nil {
		return form, err
	} else {
		form.Password = encryptedObject
	}
	return form, nil
}
