package main

// utils module
//
// Copyright (c) 2023 - Valentin Kuznetsov <vkuznet@gmail.com>
//
import (
	"log"
	"os"
	"strings"

	authz "github.com/CHESSComputing/golib/authz"
	srvConfig "github.com/CHESSComputing/golib/config"
)

// helper function to get host domain
func domain() string {
	domain := "localhost"
	hostname, err := os.Hostname()
	if err != nil {
		log.Println("ERROR: unable to get hostname, error:", err)
	}
	if !strings.Contains(hostname, ".") {
		hostname = "localhost"
	} else {
		arr := strings.Split(hostname, ".")
		domain = strings.Join(arr[len(arr)-2:], ".")
	}
	return domain
}

// helper function to get new token for given user and scope
func newToken(user, scope string) (string, error) {
	customClaims := authz.CustomClaims{User: user, Scope: scope, Kind: "client_credentials", Application: "FOXDEN"}
	duration := srvConfig.Config.Authz.TokenExpires
	if duration == 0 {
		duration = 7200
	}
	return authz.JWTAccessToken(srvConfig.Config.Authz.ClientID, duration, customClaims)
}
