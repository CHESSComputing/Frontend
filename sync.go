package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"

	srvConfig "github.com/CHESSComputing/golib/config"
)

// helper function to get sync recods from metadata services
func getSyncRecords(suuid string) ([]map[string]any, error) {
	var records []map[string]any
	// fetch records matching our did
	_httpReadRequest.GetToken()
	rurl := fmt.Sprintf("%s/records", srvConfig.Config.Services.SyncServiceURL)
	if suuid != "" {
		rurl = fmt.Sprintf("%s/record/%s", srvConfig.Config.Services.SyncServiceURL, suuid)
	}
	resp, err := _httpReadRequest.Get(rurl)
	if err != nil {
		log.Println("ERROR: unable to GET to MetaData service, error", err)
		return records, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("ERROR: unable to read response body, error", err)
		return records, err
	}
	err = json.Unmarshal(data, &records)
	if err != nil {
		log.Println("ERROR: unable to unmarshal service response, error", err)
		return records, err
	}
	return records, err
}

// helper function to delete sync record
func deleteSyncRecord(suuid string) error {
	// fetch records matching our did
	_httpDeleteRequest.GetToken()
	rurl := fmt.Sprintf("%s/record/%s", srvConfig.Config.Services.SyncServiceURL, suuid)
	log.Println("DELETE", rurl, _httpDeleteRequest)
	resp, err := _httpDeleteRequest.Delete(rurl, "application/json", bytes.NewBuffer([]byte("")))
	log.Println("DELETE response", resp, err)
	defer resp.Body.Close()
	if err != nil {
		log.Println("ERROR: unable to GET to MetaData service, error", err)
		return err
	}
	if resp.StatusCode != 200 {
		msg := fmt.Sprintf("unable to delete sync record %s", suuid)
		log.Println("ERROR: " + msg)
		return errors.New(msg)
	}
	return nil
}
