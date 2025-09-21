package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strings"

	srvConfig "github.com/CHESSComputing/golib/config"
	server "github.com/CHESSComputing/golib/server"
	services "github.com/CHESSComputing/golib/services"
	"github.com/gin-gonic/gin"
)

// helper function to clean query
func cleanQuery(query string) string {
	q := strings.Replace(query, "\r", "", -1)
	q = strings.Replace(query, "\n", "", -1)
	return q
}

// check if query is valid JSON
func validJSON(query string) error {
	if !strings.Contains(query, "{") {
		return nil
	}
	var data map[string]any
	err := json.Unmarshal([]byte(query), &data)
	return err
}

// helper function to process service request
func processResults(c *gin.Context, rec services.ServiceRequest, user string, idx, limit int, btrs []string) {
	tmpl := server.MakeTmpl(StaticFs, "Search")
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	log.Printf("service request record\n%s", rec.String())
	query := cleanQuery(rec.ServiceQuery.Query)
	tmpl["Query"] = query
	err1 := validJSON(query)
	data, err2 := json.Marshal(rec)
	if err1 != nil || err2 != nil {
		tmpl["FixQuery"] = query
		msg := "Given query is not valid JSON, error: "
		err := err1
		if err1 != nil {
			msg += err1.Error()
		} else if err2 != nil {
			msg += err2.Error()
			err = err2
		}
		tmpl["Content"] = msg
		page := server.TmplPage(StaticFs, "query_error.tmpl", tmpl)
		msg = string(template.HTML(page))
		handleError(c, http.StatusBadRequest, msg, err)
		return
	}
	// search request to DataDiscovery service
	_httpReadRequest.GetToken()
	rurl := fmt.Sprintf("%s/search", srvConfig.Config.Services.DiscoveryURL)
	resp, err := _httpReadRequest.Post(rurl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		msg := "unable to get meta-data from upstream server"
		handleError(c, http.StatusInternalServerError, msg, err)
		return
	}
	// parse data records from meta-data service
	var response services.ServiceResponse
	defer resp.Body.Close()
	data, err = io.ReadAll(resp.Body)
	if err != nil {
		content := errorTmpl(c, "unable to read response body, error", err)
		handleError(c, http.StatusBadRequest, content, err)
		return
	}

	err = json.Unmarshal(data, &response)
	if err != nil {
		content := errorTmpl(c, "unable to unmarshal response, error", err)
		handleError(c, http.StatusBadRequest, content, err)
		return
	}
	if Verbose > 1 {
		log.Printf("meta-data response\n%+v", response)
	}
	// return respose JSON if requested
	if c.Request.Header.Get("Accept") == "application/json" {
		c.JSON(http.StatusOK, response)
		return
	}

	// otherwise create proper HTML
	if response.Results.NRecords == 0 {
		tmpl["Content"] = fmt.Sprintf("No records found for your query:\n<pre>%s</pre>", query)
		page := server.TmplPage(StaticFs, "noresults.tmpl", tmpl)
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+page+footerEmpty()))
		return
	}
	records := response.Results.Records
	nrecords := response.Results.NRecords
	// extract userAttrs cookies which list which attributes to show in a record
	var attrs2show []string
	if cookie, err := c.Request.Cookie("userAttrs"); err == nil {
		for _, v := range strings.Split(cookie.Value, ",") {
			attrs2show = append(attrs2show, strings.Trim(v, " "))
		}
	}

	content := records2html(user, records, attrs2show)
	tmpl["Records"] = template.HTML(content)

	sortKey := "date"
	if len(rec.ServiceQuery.SortKeys) > 0 {
		sortKey = rec.ServiceQuery.SortKeys[0]
	}
	sortOrder := "descending"
	if rec.ServiceQuery.SortOrder == 1 {
		sortOrder = "ascending"
	}
	pages := pagination(c, query, nrecords, idx, limit, sortKey, sortOrder, btrs)
	tmpl["Pagination"] = template.HTML(pages)

	page := server.TmplPage(StaticFs, "records.tmpl", tmpl)
	// we will not use footer() on handlers since user may expand records
	// instead we'll use footerEmpty() function
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+page+footerEmpty()))
}

func findMetadataRecord(did string) (map[string]any, error) {
	var record map[string]any
	spec := make(map[string]any)
	spec["did"] = did
	rec := services.ServiceRequest{
		Client:       "frontend",
		ServiceQuery: services.ServiceQuery{Spec: spec},
	}
	data, err := json.Marshal(rec)
	if err != nil {
		msg := "unable to get meta-data from upstream server"
		return record, errors.New(msg)
	}
	// obtain valid token for read request
	_httpReadRequest.GetToken()
	rurl := fmt.Sprintf("%s/search", srvConfig.Config.Services.MetaDataURL)
	resp, err := _httpReadRequest.Post(rurl, "application/json", bytes.NewBuffer(data))
	if err != nil {
		msg := "unable to get meta-data from upstream server"
		return record, errors.New(msg)
	}
	var records []map[string]any
	defer resp.Body.Close()
	data, err = io.ReadAll(resp.Body)
	if err != nil {
		msg := "unable to read response body"
		return record, errors.New(msg)
	}
	err = json.Unmarshal(data, &records)
	if err != nil {
		msg := "unable to unmarshal response from Metadata service"
		return record, errors.New(msg)
	}
	if len(records) > 1 {
		msg := fmt.Sprintf("multiple records found for did=%s, records=%+v", did, records)
		return record, errors.New(msg)
	} else if len(records) == 0 {
		msg := fmt.Sprintf("no metadata record found for did=%s, records=%+v", did, records)
		return record, errors.New(msg)
	}

	return records[0], nil
}

// helper function to update metadata record
func updateMetadataRecord(did string, rec map[string]any) error {
	// obtain valid token for write request
	_httpWriteRequest.GetToken()
	schema := recValue(rec, "schema")
	mrec := services.MetaRecord{Schema: schema, Record: rec}
	// serialize data record
	data, err := json.Marshal(mrec)
	if err != nil {
		return err
	}
	// place request to Metadata service
	rurl := fmt.Sprintf("%s", srvConfig.Config.Services.MetaDataURL)
	resp, err := _httpWriteRequest.Put(rurl, "application/json", bytes.NewBuffer(data))
	if err != nil || resp.StatusCode != 200 {
		msg := "unable to update metadata record in FOXDEN server"
		// read response body and add it to the message
		defer resp.Body.Close()
		if data, err = io.ReadAll(resp.Body); err == nil {
			msg = fmt.Sprintf("%s, %s", msg, string(data))
		}
		log.Printf("ERROR: %s, response %+v, err %v", msg, resp, err)
		return errors.New(msg)
	}
	return nil
}
