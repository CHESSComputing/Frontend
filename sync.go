package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"

	srvConfig "github.com/CHESSComputing/golib/config"
)

// helper function to get sync recods from metadata services
func getSyncRecords() ([]map[string]any, error) {
	var records []map[string]any
	// fetch records matching our did
	_httpReadRequest.GetToken()
	rurl := fmt.Sprintf("%s/sync/records", srvConfig.Config.Services.SyncServiceURL)
	resp, err := _httpReadRequest.Get(rurl)
	defer resp.Body.Close()
	if err != nil {
		log.Println("ERROR: unable to GET to MetaData service, error", err)
		return records, err
	}
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
