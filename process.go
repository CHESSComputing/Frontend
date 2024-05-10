package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"

	srvConfig "github.com/CHESSComputing/golib/config"
	server "github.com/CHESSComputing/golib/server"
	services "github.com/CHESSComputing/golib/services"
	"github.com/gin-gonic/gin"
)

func processResults(c *gin.Context, rec services.ServiceRequest, user string, idx, limit int) {
	tmpl := server.MakeTmpl(StaticFs, "Search")
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	log.Printf("service request record\n%s", rec.String())
	query := rec.ServiceQuery.Query
	data, err := json.Marshal(rec)
	if err != nil {
		msg := "unable to parse user query"
		handleError(c, http.StatusInternalServerError, msg, err)
		return
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
		tmpl["Content"] = fmt.Sprintf("No record found for your query '%s'", query)
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
