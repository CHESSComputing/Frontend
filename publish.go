package main

import (
	"errors"
	"fmt"
	"log"
	"strings"

	materialCommons "github.com/CHESSComputing/golib/MaterialCommons"
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
	log.Println("call publishToZenodo", did)
	return doi, err
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
