package main

import (
	"errors"
	"fmt"
	"log"
	"strings"
)

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

func publishToZenodo(did, description string) (string, error) {
	var err error
	var doi string
	log.Println("call publishToZenodo", did)
	return doi, err
}

func publishToMaterialCommons(did, description string) (string, error) {
	var err error
	var doi string
	log.Println("call publishToMaterialCommons", did)
	return doi, err
}

func updateMetaDataDOI(did, doi string) error {
	var err error
	return err
}
