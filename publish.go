package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	materialCommons "github.com/CHESSComputing/golib/MaterialCommons"
	srvConfig "github.com/CHESSComputing/golib/config"
	"github.com/CHESSComputing/golib/zenodo"
)

// helper function to publish did with given provider
func publishDataset(did, provider, description string) (string, error) {
	p := strings.ToLower(provider)
	var err error
	var doi string
	if p == "zenodo" {
		doi, err = publishToZenodo(did, description)
	} else if p == "materialcommonts" {
		doi, err = publishToMaterialCommons(did, description)
	} else {
		msg := fmt.Sprintf("Provider '%s' is not supported", provider)
		return "", errors.New(msg)
	}
	return doi, err
}

// helper function to publish did to Zenodo
func publishToZenodo(did, description string) (string, error) {
	var err error
	var doi string

	// prepare http writer
	_httpWriteRequest.GetToken()

	// create new DOI resource
	rurl := fmt.Sprintf("%s/create", srvConfig.Config.Services.PublicationURL)
	resp, err := _httpWriteRequest.Post(rurl, "application/json", bytes.NewBuffer([]byte{}))
	defer resp.Body.Close()
	if err != nil {
		return doi, err
	}

	// capture response and extract document id (did)
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return doi, err
	}
	var doc zenodo.CreateResponse
	err = json.Unmarshal(data, &doc)
	if err != nil {
		return doi, err
	}
	docId := doc.Id

	// create new meta-data record
	creator := zenodo.Creator{Name: "FOXDEN", Affiliation: "Cornell University"}
	mrec := zenodo.MetaDataRecord{
		PublicationType: "dataset",
		Description:     description,
		Title:           fmt.Sprintf("FOXDEN dataset did=%s", did),
		Licences:        []string{"MIT"},
		Creators:        []zenodo.Creator{creator},
	}
	data, err = json.Marshal(mrec)
	if err != nil {
		return doi, err
	}
	rurl = fmt.Sprintf("%s/update/%d", srvConfig.Config.Services.PublicationURL, docId)
	metaResp, err := _httpWriteRequest.Put(rurl, "application/json", bytes.NewBuffer(data))
	defer metaResp.Body.Close()
	if err != nil || metaResp.StatusCode != 200 {
		return doi, err
	}

	// publish the record
	rurl = fmt.Sprintf("%s/publish/%d", srvConfig.Config.Services.PublicationURL, docId)
	publishResp, err := _httpWriteRequest.Post(rurl, "application/json", bytes.NewBuffer([]byte{}))
	defer publishResp.Body.Close()
	if err != nil || (publishResp.StatusCode < 200 || publishResp.StatusCode >= 400) {
		return doi, err
	}

	// fetch our document
	rurl = fmt.Sprintf("%s/docs/%d", srvConfig.Config.Services.PublicationURL, docId)
	docsResp, err := _httpReadRequest.Get(rurl)
	defer docsResp.Body.Close()
	if err != nil || (docsResp.StatusCode < 200 || docsResp.StatusCode >= 400) {
		return doi, err
	}
	data, err = io.ReadAll(docsResp.Body)
	if err != nil {
		return doi, err
	}

	// parse doi record
	var doiRecord zenodo.DoiRecord
	err = json.Unmarshal(data, &doiRecord)
	if err != nil {
		return doi, err
	}
	return doiRecord.Doi, err
}

// helper function to publish did into MaterialCommons
func publishToMaterialCommons(did, description string) (string, error) {
	doi, err := materialCommons.Publish(did, description)
	return doi, err
}

// helper function to update DOI information in FOXDEN MetaData service
func updateMetaDataDOI(did, doi string) error {
	var err error
	return err
}
