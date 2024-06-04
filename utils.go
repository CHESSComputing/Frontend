package main

// utils module
//
// Copyright (c) 2023 - Valentin Kuznetsov <vkuznet@gmail.com>
//
import (
	"encoding/json"
	"fmt"
	"io"
	"log"

	authz "github.com/CHESSComputing/golib/authz"
	srvConfig "github.com/CHESSComputing/golib/config"
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

// helper function to get provenance data
func getData(api, did string) ([]map[string]any, error) {
	var records []map[string]any
	// search request to DataDiscovery service
	rurl := fmt.Sprintf("%s/%s?did=%s", srvConfig.Config.Services.DataBookkeepingURL, api, did)
	resp, err := _httpReadRequest.Get(rurl)
	if err != nil {
		return records, err
	}
	// parse data records from meta-data service
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return records, err
	}
	if Verbose > 0 {
		log.Println("provenance data\n", string(data))
	}
	err = json.Unmarshal(data, &records)
	return records, err
}
