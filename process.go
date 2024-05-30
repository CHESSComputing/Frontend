package main

import (
	"bytes"
	"encoding/json"
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
func processResults(c *gin.Context, rec services.ServiceRequest, user string, idx, limit int) {
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
	}
	// search request to DataDiscovery service
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
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}
	err = json.Unmarshal(data, &response)
	if err != nil {
		content := errorTmpl(c, "unable to unmarshal response, error", err)
		c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(header()+content+footer()))
		return
	}
	if Verbose > 1 {
		log.Printf("meta-data response\n%+v", response)
	}
	if response.Results.NRecords == 0 {
		tmpl["Content"] = fmt.Sprintf("No records found for your query:\n<pre>%s</pre>", query)
		page := server.TmplPage(StaticFs, "noresults.tmpl", tmpl)
		c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+page+footerEmpty()))
		return
	}
	records := response.Results.Records
	nrecords := response.Results.NRecords
	content := records2html(user, records)
	tmpl["Records"] = template.HTML(content)

	pages := pagination(c, query, nrecords, idx, limit)
	tmpl["Pagination"] = template.HTML(pages)

	page := server.TmplPage(StaticFs, "records.tmpl", tmpl)
	// we will not use footer() on handlers since user may expand records
	// instead we'll use footerEmpty() function
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+page+footerEmpty()))
	// c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(header()+page+footer()))
}
