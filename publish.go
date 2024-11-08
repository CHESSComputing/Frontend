package main

import (
	"errors"
	"fmt"
	"strings"

	materialCommons "github.com/CHESSComputing/golib/MaterialCommons"
	"github.com/CHESSComputing/golib/zenodo"
)

// helper function to publish did with given provider
func publishDataset(did, provider, description string) (string, error) {
	p := strings.ToLower(provider)
	var err error
	var doi string
	if p == "zenodo" {
		doi, err = publishToZenodo(did, description)
	} else if p == "materialcommons" {
		doi, err = publishToMaterialCommons(did, description)
	} else {
		msg := fmt.Sprintf("Provider '%s' is not supported", provider)
		return "", errors.New(msg)
	}
	return doi, err
}

// helper function to publish did to Zenodo
func publishToZenodo(did, description string) (string, error) {
	var doi string
	var err error
	docId, err := zenodo.CreateRecord()
	if err != nil {
		return doi, err
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
		return doi, err
	}

	// publish record
	doiRecord, err := zenodo.PublishRecord(docId)
	if err != nil {
		return doi, err
	}
	return doiRecord.Doi, nil
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
