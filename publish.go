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

	materialCommons "github.com/CHESSComputing/golib/MaterialCommons"
	srvConfig "github.com/CHESSComputing/golib/config"
	services "github.com/CHESSComputing/golib/services"
	"github.com/CHESSComputing/golib/zenodo"
)

// helper function to publish did with given provider
func publishDataset(did, provider, description string) (string, string, error) {
	p := strings.ToLower(provider)
	var err error
	var doi, doiLink string
	if p == "zenodo" {
		doi, doiLink, err = publishToZenodo(did, description)
	} else if p == "materialcommons" {
		doi, doiLink, err = publishToMaterialCommons(did, description)
	} else {
		msg := fmt.Sprintf("Provider '%s' is not supported", provider)
		err = errors.New(msg)
	}
	return doi, doiLink, err
}

// helper function to publish did to Zenodo
func publishToZenodo(did, description string) (string, string, error) {
	var doi, doiLink string
	var err error
	docId, err := zenodo.CreateRecord()
	if err != nil {
		return doi, doiLink, err
	}

	// create new meta-data record
	creator := zenodo.Creator{Name: "FOXDEN", Affiliation: "Cornell University"}
	mrec := zenodo.MetaDataRecord{
		PublicationType: "dataset",
		Description:     description,
		Title:           fmt.Sprintf("FOXDEN dataset did=%s", did),
		Licences:        []string{"MIT"},
		Creators:        []zenodo.Creator{creator},
	}
	err = zenodo.UpdateRecord(docId, mrec)
	if err != nil {
		return doi, doiLink, err
	}

	// publish record
	doiRecord, err := zenodo.PublishRecord(docId)
	if err != nil {
		return doi, doiLink, err
	}
	return doiRecord.Doi, doiRecord.DoiUrl, nil
}

// helper function to publish did into MaterialCommons
func publishToMaterialCommons(did, description string) (string, string, error) {
	doi, doiLink, err := materialCommons.Publish(did, description)
	return doi, doiLink, err
}

// helper function to update DOI information in FOXDEN MetaData service
func updateMetaDataDOI(did, doi, doiLink string) error {
	var err error

	// extract schema from did
	var schema string
	for _, part := range strings.Split(did, "/") {
		if strings.HasPrefix(part, "beamline=") {
			schema = strings.Replace(part, "beamline=", "", -1)
			break
		}
	}
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

		// create meta-data record for update
		mrec := services.MetaRecord{Schema: schema, Record: rec}

		// prepare http writer
		_httpWriteRequest.GetToken()

		// place request to MetaData service
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
