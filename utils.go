package main

// utils module
//
// Copyright (c) 2023 - Valentin Kuznetsov <vkuznet@gmail.com>
//
import (
	"encoding/json"
	"io"
	"os"

	authz "github.com/CHESSComputing/golib/authz"
	srvConfig "github.com/CHESSComputing/golib/config"
	utils "github.com/CHESSComputing/golib/utils"
)

// helper function to get new token for given user and scope
func newToken(user, scope string) (string, error) {
	customClaims := authz.CustomClaims{User: user, Scope: scope, Kind: "client_credentials", Application: "FOXDEN"}
	duration := srvConfig.Config.Authz.TokenExpires
	if duration == 0 {
		duration = 7200
	}
	return authz.JWTAccessToken(srvConfig.Config.Authz.ClientID, duration, customClaims)
}

// helper function to get all FOXDEN QL keys
func qlKeys() ([]string, error) {
	var keys []string
	fname := srvConfig.Config.QL.ServiceMapFile
	file, err := os.Open(fname)
	if err != nil {
		return keys, err
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return keys, err
	}
	var smap map[string][]string
	err = json.Unmarshal(data, &smap)
	if err != nil {
		return keys, err
	}
	var allKeys []string
	for _, keys := range smap {
		for _, key := range keys {
			allKeys = append(allKeys, key)
		}
	}
	keys = utils.List2Set[string](allKeys)
	return keys, nil
}
