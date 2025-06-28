package main

// utils module
//
// Copyright (c) 2023 - Valentin Kuznetsov <vkuznet@gmail.com>
//
import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strings"

	authz "github.com/CHESSComputing/golib/authz"
	srvConfig "github.com/CHESSComputing/golib/config"
	ldap "github.com/CHESSComputing/golib/ldap"
	services "github.com/CHESSComputing/golib/services"
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

// columnNames converts JSON attributes to column names
func columnNames(attrs []string) []string {
	var out []string
	for _, attr := range attrs {
		var camel string
		words := strings.Split(attr, "_")
		for _, word := range words {
			camel += strings.Title(word)
		}
		out = append(out, camel)
	}
	return out
}

// helper function to obtain chunk of records for given service request
func numberOfRecords(rec services.ServiceRequest) (int, error) {
	var total int

	// obtain valid token
	_httpReadRequest.GetToken()

	// based on user query process request from all FOXDEN services
	data, err := json.Marshal(rec)
	if err != nil {
		log.Println("ERROR: marshall error", err)
		return total, err
	}
	rurl := fmt.Sprintf("%s/nrecords", srvConfig.Config.Services.DiscoveryURL)
	resp, err := _httpReadRequest.Post(rurl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Println("ERROR: HTTP POST error", err)
		return total, err
	}
	// parse data records from discovery service
	defer resp.Body.Close()
	data, err = io.ReadAll(resp.Body)
	if err != nil {
		log.Println("ERROR: IO error", err)
		return total, err
	}
	var response services.ServiceResponse
	err = json.Unmarshal(data, &response)
	if err != nil {
		log.Println("ERROR: unable to unmarshal response", err)
		return total, err
	}
	if response.HttpCode != http.StatusOK {
		log.Println("ERROR", response.Error)
		return 0, err
	}
	return response.Results.NRecords, nil
}

// helper function to obtain chunk of records for given service request
func chunkOfRecords(rec services.ServiceRequest) (services.ServiceResponse, error) {
	var response services.ServiceResponse

	// obtain valid token
	_httpReadRequest.GetToken()

	// based on user query process request from all FOXDEN services
	data, err := json.Marshal(rec)
	if err != nil {
		log.Println("ERROR: marshall error", err)
		return response, err
	}
	rurl := fmt.Sprintf("%s/search", srvConfig.Config.Services.DiscoveryURL)
	resp, err := _httpReadRequest.Post(rurl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		log.Println("ERROR: HTTP POST error", err)
		return response, err
	}
	// parse data records from discovery service
	defer resp.Body.Close()
	data, err = io.ReadAll(resp.Body)
	if err != nil {
		log.Println("ERROR: IO error", err)
		return response, err
	}
	err = json.Unmarshal(data, &response)
	return response, err
}

// helper function to make new query out of search filter and list of attributes
func makeSpec(searchFilter string, attrs []string) map[string]any {
	if srvConfig.Config.Embed.DocDb != "" {
		// TODO: so far for embed db we can't use filters
		return map[string]any{}
	}
	var filters []map[string]any
	for _, attr := range attrs {
		if pat, err := regexp.Compile(fmt.Sprintf(".*%s.*", searchFilter)); err == nil {
			filters = append(filters, map[string]any{
				attr: map[string]any{"$regex": pat},
			})
		}
	}
	spec := map[string]any{
		"$or": filters,
	}
	return spec
}

// helper function to update spec with ldap attributes. It has the following logic
// - in case of search spec we only update input spec with btrs limited to user ldap attributes
// - in case of filter spec we make a new spec based on filter conditions
func updateSpec(ispec map[string]any, attrs ldap.Entry, useCase string) map[string]any {
	if (len(attrs.Foxdens) > 0 && srvConfig.Config.Frontend.CheckAdmins) ||
		srvConfig.Config.Frontend.AllowAllRecords {
		// foxden attributes allows to see all btrs
		return ispec
	}

	// search use-case
	if useCase == "search" {
		// check if ispec contains btrs and make final list from attrs.Btrs
		// this will restrict spec to btrs allowed by ldap entry btrs associated with user
		if btrs, ok := ispec["btr"]; ok {
			ispec["btr"] = map[string]any{"$in": finalBtrs(btrs, attrs.Btrs)}
		} else if len(attrs.Btrs) != 0 {
			ispec["btr"] = map[string]any{"$in": attrs.Btrs}
		}
		return ispec
	}

	// filter use-case
	var filters []map[string]any
	if val, ok := ispec["$or"]; ok {
		specFilters := val.([]map[string]any)
		// we already have series of maps with or condition
		for _, flt := range specFilters {
			if _, ok := flt["btr"]; ok {
				continue
			}
			filters = append(filters, flt)
		}
	} else {
		// ispec is plain dictionary of key:value pairs without $or condition
		for key, val := range ispec {
			if key == "btr" {
				continue
			}
			flt := map[string]any{
				key: val,
			}
			filters = append(filters, flt)
		}
	}
	// default spec will contain only btrs
	spec := map[string]any{"btr": map[string]any{"$in": attrs.Btrs}}
	if len(filters) > 0 {
		// if we had other filters we will construct "$and" query with them
		spec = map[string]any{
			"$and": []map[string]any{
				map[string]any{"$or": filters},
				map[string]any{"btr": map[string]any{"$in": attrs.Btrs}},
			},
		}
	}
	return spec
}

// helper function to get final list of btrs
func finalBtrs(btrs any, attrBtrs []string) []string {
	validBtrs := make(map[string]struct{}) // Use map to avoid duplicates
	attrSet := make(map[string]struct{})

	// Convert attrBtrs slice into a set for fast lookup
	for _, attr := range attrBtrs {
		attrSet[attr] = struct{}{}
	}

	// Helper function to add values if they exist in attrBtrs
	addIfValid := func(value string) {
		if _, exists := attrSet[value]; exists {
			validBtrs[value] = struct{}{}
		}
	}

	// Process different types of `btrs`
	switch v := btrs.(type) {
	case string:
		addIfValid(v)
	case []string:
		for _, item := range v {
			addIfValid(item)
		}
	case map[string]any:
		// Handle {"$or": [...] } and {"$in": [...] }
		for key, val := range v {
			if key == "$or" || key == "$in" {
				if list, ok := val.([]any); ok {
					for _, item := range list {
						if str, ok := item.(string); ok {
							addIfValid(str)
						}
					}
				}
			}
		}
	}

	// Convert map keys to slice
	result := make([]string, 0, len(validBtrs))
	for key := range validBtrs {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
