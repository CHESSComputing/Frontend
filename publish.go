package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	srvConfig "github.com/CHESSComputing/golib/config"
	srvDoi "github.com/CHESSComputing/golib/doi"
	services "github.com/CHESSComputing/golib/services"
)

// pointers to various doi providers
var zenodoDoi *srvDoi.ZenodoProvider
var mcDoi *srvDoi.MCProvider
var dataciteDoi *srvDoi.DataciteProvider

func getMetaData(user, did string) (map[string]any, error) {
	var rec map[string]any
	token, err := newToken(user, "read")
	if err != nil {
		return rec, err
	}
	_httpReadRequest.Token = token
	query := fmt.Sprintf("{\"did\": \"%s\"}", did)
	srec := services.ServiceRequest{
		Client:       "foxden-doi",
		ServiceQuery: services.ServiceQuery{Query: query, Idx: 0, Limit: -1},
	}

	data, err := json.Marshal(srec)
	rurl := fmt.Sprintf("%s/search", srvConfig.Config.Services.MetaDataURL)
	resp, err := _httpReadRequest.Post(rurl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return rec, err
	}
	defer resp.Body.Close()
	data, err = io.ReadAll(resp.Body)
	if err != nil {
		return rec, err
	}
	var records []map[string]any
	err = json.Unmarshal(data, &records)
	if err != nil {
		return rec, err
	}
	if len(records) != 1 {
		return rec, errors.New("wrong number of records")
	}
	rec = records[0]
	return rec, nil
}

// helper function to publish did with given provider
func publishDataset(user, provider, did, description string, parents []string, doiPublic bool) (string, string, error) {

	// get meta-data record associated with did
	record, err := getMetaData(user, did)
	if err != nil {
		return "", "", err
	}
	if len(parents) > 0 {
		record["doi_parents_dids"] = parents
	}

	if val, ok := record["doi"]; ok {
		if fmt.Sprintf("%v", val) != "" {
			msg := fmt.Sprintf("Record with did=%s has already DOI: %s", did, val)
			return "", "", errors.New(msg)
		}
	}
	p := strings.ToLower(provider)
	var doi, doiLink string
	if p == "zenodo" {
		if zenodoDoi == nil {
			zenodoDoi = &srvDoi.ZenodoProvider{Verbose: srvConfig.Config.Frontend.WebServer.Verbose}
		}
		zenodoDoi.Init()
		doi, doiLink, err = zenodoDoi.Publish(did, description, record, doiPublic)
	} else if p == "materialscommons" {
		if mcDoi == nil {
			mcDoi = &srvDoi.MCProvider{Verbose: srvConfig.Config.Frontend.WebServer.Verbose}
		}
		mcDoi.Init()
		doi, doiLink, err = mcDoi.Publish(did, description, record, doiPublic)
	} else if p == "datacite" {
		if dataciteDoi == nil {
			dataciteDoi = &srvDoi.DataciteProvider{Verbose: srvConfig.Config.Frontend.WebServer.Verbose}
		}
		dataciteDoi.Init()
		doi, doiLink, err = dataciteDoi.Publish(did, description, record, doiPublic)
	} else {
		msg := fmt.Sprintf("Provider '%s' is not supported", provider)
		err = errors.New(msg)
	}
	if err != nil {
		log.Printf("ERROR: unable to publish did=%s provider=%s error=%v", did, p, err)
		return doi, doiLink, err
	}
	return doi, doiLink, err
}

// helper function to update DOI information in FOXDEN MetaData service
func updateMetaDataDOI(user, did, schema, doiProvider, doi, doiLink string, doiPublic bool, doiAccessMetadata string) error {
	var err error

	if strings.Contains(schema, ",") {
		msg := fmt.Sprintf("unsupported did=%s with multiple schemas %s for MetaData update", did, schema)
		return errors.New(msg)
	}

	// fetch records matching our did
	_httpReadRequest.GetToken()
	rurl := fmt.Sprintf("%s/record?did=%s", srvConfig.Config.Services.MetaDataURL, did)
	resp, err := _httpReadRequest.Get(rurl)
	defer resp.Body.Close()
	if err != nil {
		log.Println("ERROR: unable to GET to MetaData service, error", err)
		return err
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("ERROR: unable to read response body, error", err)
		return err
	}
	var records []map[string]any
	err = json.Unmarshal(data, &records)
	if err != nil {
		log.Println("ERROR: unable to unmarshal service response, error", err)
		return err
	}

	// for all matching records perform update
	for _, rec := range records {
		// drop _id as it does not belong to the meta-data schema
		delete(rec, "_id")
		// and add doi attributes
		rec["doi"] = doi
		rec["doi_url"] = doiLink
		rec["doi_user"] = user
		rec["doi_public"] = doiPublic
		rec["doi_provider"] = doiProvider
		rec["doi_created_at"] = time.Now().Format(time.RFC3339)
		if doiAccessMetadata == "on" {
			rec["doi_access_metadata"] = true
		} else {
			rec["doi_access_metadata"] = false
		}
		// if we run our own DOI Service we need to use our permanent doiLink
		if srvConfig.Config.DOIServiceURL != "" {
			foxdenDoiLink := fmt.Sprintf("%s/doi/%s", srvConfig.Config.DOIServiceURL, doi)
			rec["doi_foxden_url"] = foxdenDoiLink
		}

		// create meta-data record for update
		mrec := services.MetaRecord{Schema: schema, Record: rec}

		// prepare http writer
		_httpWriteRequest.GetToken()

		// place request to MetaData service to update record with doi info
		rurl := fmt.Sprintf("%s", srvConfig.Config.Services.MetaDataURL)
		data, err := json.Marshal(mrec)
		if err != nil {
			log.Println("ERROR: unable to marshal meta-data record, error", err)
			return err
		}
		resp, err := _httpWriteRequest.Put(rurl, "application/json", bytes.NewBuffer(data))
		defer resp.Body.Close()
		if err != nil {
			log.Println("ERROR: unable to POST to MetaData service, error", err)
			return err
		}
		data, err = io.ReadAll(resp.Body)
		if err != nil {
			log.Println("ERROR: unable to read response body, error", err)
			return err
		}

		var sresp services.ServiceResponse
		err = json.Unmarshal(data, &sresp)
		if err != nil {
			log.Println("ERROR: unable to unmarshal service response, error", err)
			return err
		}
		if sresp.SrvCode != 0 || sresp.HttpCode != http.StatusOK {
			return errors.New(sresp.String())
		}
	}
	return nil
}

// helper function to make DOI public for given provider
func makePublic(doi, provider string) error {
	var err error
	p := strings.ToLower(provider)
	if p == "zenodo" {
		if zenodoDoi == nil {
			zenodoDoi = &srvDoi.ZenodoProvider{Verbose: srvConfig.Config.Frontend.WebServer.Verbose}
		}
		zenodoDoi.Init()
		err = zenodoDoi.MakePublic(doi)
	} else if p == "materialscommons" {
		if mcDoi == nil {
			mcDoi = &srvDoi.MCProvider{Verbose: srvConfig.Config.Frontend.WebServer.Verbose}
		}
		mcDoi.Init()
		err = mcDoi.MakePublic(doi)
	} else if p == "datacite" {
		if dataciteDoi == nil {
			dataciteDoi = &srvDoi.DataciteProvider{Verbose: srvConfig.Config.Frontend.WebServer.Verbose}
		}
		dataciteDoi.Init()
		err = dataciteDoi.MakePublic(doi)
	} else {
		msg := fmt.Sprintf("Provider '%s' is not supported", provider)
		err = errors.New(msg)
	}
	return err
}
