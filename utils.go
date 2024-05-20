package main

// utils module
//
// Copyright (c) 2023 - Valentin Kuznetsov <vkuznet@gmail.com>
//
import (
	"encoding/json"
	"fmt"
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

// QLKey defines structure of QL key
type QLKey struct {
	Key         string `json:"key"`
	Description string `json:"description,omitempty"`
	Service     string `json:"service"`
	Units       string `json:"units,omitempty"`
	Schema      string `json:"schema,omitempty"`
	DataType    string `json:"type"`
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
	var arr []QLKey
	err = json.Unmarshal(data, &arr)
	if err != nil {
		return keys, err
	}
	var allKeys []string
	for _, elem := range arr {
		// each qmap here is QLKey structure
		desc := elem.Description
		if desc == "" {
			desc = "description not available"
		}
		srv := fmt.Sprintf("%s:%s", elem.Service, elem.Schema)
		if elem.Schema == "" {
			srv = elem.Service
		}
		key := fmt.Sprintf("%s: (%s) %s", elem.Key, srv, desc)
		if elem.Units != "" {
			key += fmt.Sprintf(", units:%s", elem.Units)
		}
		if elem.DataType != "" {
			key += fmt.Sprintf(", data-type:%s", elem.DataType)
		}
		allKeys = append(allKeys, key)
	}
	keys = utils.List2Set[string](allKeys)
	return keys, nil
}
