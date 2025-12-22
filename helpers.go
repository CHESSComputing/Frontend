package main

// helpers module
//
// Copyright (c) 2023 - Valentin Kuznetsov <vkuznet@gmail.com>
//
import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/url"
	"sort"
	"strconv"
	"strings"

	beamlines "github.com/CHESSComputing/golib/beamlines"
	srvConfig "github.com/CHESSComputing/golib/config"
	schema "github.com/CHESSComputing/golib/schema"
	server "github.com/CHESSComputing/golib/server"
	utils "github.com/CHESSComputing/golib/utils"
	"github.com/gin-gonic/gin"
)

//
// helper functions
//

// helper function to provide error page
func handleError(c *gin.Context, code int, msg string, err error) {
	page := server.ErrorPage(StaticFs, msg, err)
	if c.Request.Header.Get("Accept") == "application/json" {
		c.JSON(code, gin.H{"error": err.Error(), "message": msg, "code": code})
		return
	}
	c.Data(code, "text/html; charset=utf-8", []byte(header()+page+footer()))
}

// helper function to provides error template message
func errorTmpl(c *gin.Context, msg string, err error) string {
	tmpl := server.MakeTmpl(StaticFs, "Status")
	tmpl["Content"] = template.HTML(fmt.Sprintf("<div>%s</div>\n<br/><h3>ERROR</h3>%v", msg, err))
	content := server.TmplPage(StaticFs, "error.tmpl", tmpl)
	return content
}

// helper functiont to provides success template message
func successTmpl(c *gin.Context, msg string) string {
	tmpl := server.MakeTmpl(StaticFs, "Status")
	tmpl["Content"] = template.HTML(fmt.Sprintf("<h3>SUCCESS</h3><div>%s</div>", msg))
	content := server.TmplPage(StaticFs, "success.tmpl", tmpl)
	return content
}

// helper funtion to get record value
func recValue(rec map[string]any, attr string) string {
	if val, ok := rec[attr]; ok {
		switch v := val.(type) {
		case float64:
			if attr == "did" {
				return fmt.Sprintf("%d", int64(val.(float64)))
			}
			return fmt.Sprintf("%f", v)
		default:
			return fmt.Sprintf("%v", v)
		}
	}
	return "Not available"
}

// helper function to get dids from DataHub service
func datahubDidHashes() []string {
	var didHashes []string
	if srvConfig.Config.DataHubURL == "" {
		return didHashes
	}
	_httpReadRequest.GetToken()
	rurl := fmt.Sprintf("%s/datahub", srvConfig.Config.DataHubURL)
	resp, err := _httpReadRequest.Get(rurl)
	if err != nil {
		log.Println("ERROR: unable to get datahub didHashes, error:", err)
		return didHashes
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("ERROR: unable to get datahub didHashes, error:", err)
		return didHashes
	}
	var arr []string
	err = json.Unmarshal(data, &arr)
	if err != nil {
		log.Println("ERROR: unable to get datahub didHashes, error:", err)
		return didHashes
	}
	for _, entry := range arr {
		if did, err := url.QueryUnescape(entry); err == nil {
			didHashes = append(didHashes, did)
		}
	}
	return didHashes
}

// helper function to prepare HTML page for given services records
func records2html(user string, records []map[string]any, attrs2show []string) string {
	var out []string
	didhashes := datahubDidHashes()
	for _, rec := range records {
		tmpl := server.MakeTmpl(StaticFs, "Record")
		tmpl["User"] = user
		tmpl["Id"] = recValue(rec, "did")
		did := recValue(rec, "did")
		tmpl["Did"] = did
		if val, err := url.QueryUnescape(did); err == nil {
			tmpl["DidEncoded"] = val
		}
		tmpl["Cycle"] = recValue(rec, "cycle")
		tmpl["Beamline"] = recValue(rec, "beamline")
		tmpl["Btr"] = recValue(rec, "btr")
		tmpl["MCProjectName"] = fmt.Sprintf("chess_btr_%s", recValue(rec, "btr"))
		tmpl["Sample"] = recValue(rec, "sample_name")
		tmpl["Schema"] = recValue(rec, "schema")
		tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
		tmpl["Record"] = rec
		tmpl["RecordTable"] = reprRecord(rec, "table")
		tmpl["RecordDescription"] = reprRecord(rec, "description")
		tmpl["RecordJSON"] = reprRecord(rec, "json")
		tmpl["Description"] = recValue(rec, "description")
		if val, err := lastModified(rec); err == nil {
			tmpl["TimeStamp"] = val
		} else {
			tmpl["TimeStamp"] = "Not Available"
		}
		if val, ok := rec["globus_link"]; ok {
			tmpl["GlobusLink"] = fmt.Sprintf("%v", val)
		}
		if val, ok := rec["doi"]; ok {
			tmpl["Doi"] = val
		}
		if val, ok := rec["schema"]; ok {
			tmpl["Schema"] = val
		}
		// first check if there is doi_url
		if val, ok := rec["doi_url"]; ok {
			tmpl["DoiLink"] = val
		}
		// then, check if we created foxden_doi_url and use it instead for web UI link
		if val, ok := rec["doi_foxden_url"]; ok {
			tmpl["DoiLink"] = val
		}
		if val, ok := rec["doi_public"]; ok {
			tmpl["DoiPublic"] = val
		}
		if val, ok := rec["doi_provider"]; ok {
			tmpl["DoiProvider"] = val
		}
		if val, ok := rec["beamline"]; ok {
			var beamlines []string
			switch vvv := val.(type) {
			case []any:
				for _, v := range vvv {
					beamlines = append(beamlines, fmt.Sprintf("%v", v))
				}
			case string:
				beamlines = append(beamlines, vvv)
			case any:
				beamlines = append(beamlines, fmt.Sprintf("%v", vvv))
			}
			for _, b := range beamlines {
				if utils.InList(b, srvConfig.Config.CHESSMetaData.SpecScanBeamlines) {
					tmpl["SpecScanLink"] = fmt.Sprintf("/specscans?did=%s", url.QueryEscape(recValue(rec, "did")))
					break
				}
			}
		}

		// look for data location attributes, if found create Data Management link
		/*
			for _, loc := range srvConfig.Config.CHESSMetaData.DataLocationAttributes {
				if _, ok := rec[loc]; ok {
					tmpl["RawDataLink"] = fmt.Sprintf("/dm?did=%s&attr=data_location_raw", recValue(rec, "did"))
					break
				}
			}
		*/
		if val, ok := rec["data_location_raw"]; ok && val != "" {
			tmpl["RawDataLink"] = fmt.Sprintf("/dm?did=%s&attr=data_location_raw", recValue(rec, "did"))
		}
		if val, ok := rec["data_location_reduced"]; ok && val != "" {
			tmpl["ReducedDataLink"] = fmt.Sprintf("/dm?did=%s&attr=data_location_reduced", recValue(rec, "did"))
		}

		// check if did hash exists in DataHub
		sum := md5.Sum([]byte(did))
		didhash := hex.EncodeToString(sum[:])
		if utils.InList(didhash, didhashes) {
			tmpl["AuxDataLink"] = fmt.Sprintf("%s/datahub/%s", srvConfig.Config.DataHubURL, didhash)
		}

		if val, ok := rec["history"]; ok {
			switch t := val.(type) {
			case []any:
				tmpl["RecordVersion"] = len(t) + 1 // human counter, i.e. if one history record it is 2nd version
			}
		}
		amap := make(map[string]any)
		for _, attr := range attrs2show {
			if val, ok := rec[attr]; ok {
				amap[attr] = val
			}
		}
		tmpl["AttributesMap"] = amap

		content := server.TmplPage(StaticFs, "record.tmpl", tmpl)
		out = append(out, content)
	}
	return strings.Join(out, "\n")
}

var _metaManager *schema.MetaDataManager

// helper function to represent record
func reprRecord(rec map[string]any, format string) string {
	sname := recValue(rec, "schema")
	umap := _metaManager.Units(sname)
	dmap := _metaManager.Descriptions(sname)
	if format == "json" {
		var srec string
		data, err := json.MarshalIndent(rec, "", "  ")
		if err != nil {
			log.Println("ERROR: unable to marshal record", rec, err)
			srec = "Not available"
		} else {
			srec = string(data)
		}
		return srec
	}
	keys := utils.MapKeys(rec)
	sort.Strings(keys)
	var maxLen int
	for _, k := range keys {
		if len(k) > maxLen {
			maxLen = len(k)
		}
	}
	var out string
	if format == "description" {
		for _, key := range keys {
			if desc, ok := dmap[key]; ok {
				out = fmt.Sprintf("%s\n%s: %v", out, utils.PaddedKey(key, maxLen), desc)
			} else {
				out = fmt.Sprintf("%s\n%s: Not Available", out, utils.PaddedKey(key, maxLen))
			}
		}
		return out
	}
	for _, key := range keys {
		val, _ := rec[key]
		if unit, ok := umap[key]; ok {
			if unit != "" {
				out = fmt.Sprintf("%s\n%s: %v (%s)", out, utils.PaddedKey(key, maxLen), val, unit)
			} else {
				out = fmt.Sprintf("%s\n%s: %v", out, utils.PaddedKey(key, maxLen), val)
			}
		} else {
			out = fmt.Sprintf("%s\n%s: %v", out, utils.PaddedKey(key, maxLen), val)
		}
	}
	return out
}

// helper function to make pagination
func pagination(c *gin.Context, query string, nres, startIdx, limit int, sortKey, sortOrder string, btrs []string) string {
	tmpl := server.MakeTmpl(StaticFs, "Search")
	if user, err := getUser(c); err == nil {
		tmpl["User"] = user
		attrs := userAttrs(user)
		tmpl["DataAttributes"] = strings.Join(attrs, ",")
	}
	eQuery := url.QueryEscape(query)
	url := fmt.Sprintf("/search?query=%s&sort_keys=%s&sort_order=%s", eQuery, sortKey, sortOrder)
	if nres > 0 {
		tmpl["StartIndex"] = fmt.Sprintf("%d", startIdx+1)
	} else {
		tmpl["StartIndex"] = fmt.Sprintf("%d", startIdx)
	}
	if nres > startIdx+limit {
		tmpl["EndIndex"] = fmt.Sprintf("%d", startIdx+limit)
	} else {
		tmpl["EndIndex"] = fmt.Sprintf("%d", nres)
	}
	tmpl["Total"] = fmt.Sprintf("%d", nres)
	tmpl["FirstUrl"] = makeURL(url, "first", startIdx, limit, nres)
	tmpl["PrevUrl"] = makeURL(url, "prev", startIdx, limit, nres)
	tmpl["NextUrl"] = makeURL(url, "next", startIdx, limit, nres)
	tmpl["LastUrl"] = makeURL(url, "last", startIdx, limit, nres)
	tmpl["Query"] = template.HTML(query)
	tmpl["SortKey"] = sortKey
	tmpl["SortOrder"] = sortOrder
	tmpl["Query"] = query
	tmpl["Btrs"] = btrs
	page := server.TmplPage(StaticFs, "pagination.tmpl", tmpl)
	return fmt.Sprintf("%s", page)
}

// helper function to make URL
func makeURL(url, urlType string, startIdx, limit, nres int) string {
	if limit < 0 {
		limit = nres
	}
	var out string
	var idx int
	if urlType == "first" {
		idx = 0
	} else if urlType == "prev" {
		if startIdx != 0 {
			idx = startIdx - limit
		} else {
			idx = 0
		}
	} else if urlType == "next" {
		idx = startIdx + limit
	} else if urlType == "last" {
		j := 0
		for i := 0; i < nres; i = i + limit {
			if i > nres {
				break
			}
			j = i
		}
		idx = j
	}
	out = fmt.Sprintf("%s&amp;idx=%d&&amp;limit=%d", url, idx, limit)
	return out
}

// helper function to generate input form
func genForm(c *gin.Context, fname string, record *map[string]any) (string, error) {
	var out []string
	val := fmt.Sprintf("<h3>Web form submission</h3><br/>")
	out = append(out, val)
	beamline := utils.FileName(fname)
	if strings.Contains(fname, "user") {
		tmpl := server.MakeTmpl(StaticFs, "Form")
		form := server.TmplPage(StaticFs, "userform.tmpl", tmpl)
		tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
		tmpl["Beamline"] = beamline
		tmpl["Description"] = ""
		tmpl["Form"] = template.HTML(form)
		if record != nil {
			if val, ok := (*record)["description"]; ok {
				tmpl["Description"] = val
			}
		}
		return server.TmplPage(StaticFs, "form_beamline.tmpl", tmpl), nil
	}
	if strings.Contains(strings.ToLower(fname), "composite") {
		tmpl := server.MakeTmpl(StaticFs, "Form")
		form := server.TmplPage(StaticFs, "composite.tmpl", tmpl)
		tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
		tmpl["Beamline"] = beamline
		tmpl["Description"] = ""
		if record != nil {
			if val, ok := (*record)["description"]; ok {
				tmpl["Description"] = val
			}
		}
		tmpl["Form"] = template.HTML(form)
		return server.TmplPage(StaticFs, "form_beamline.tmpl", tmpl), nil
	}
	val = fmt.Sprintf("<input class=\"input\" name=\"beamline\" type=\"hidden\" value=\"\"/>%s", beamline)
	schema, err := _smgr.Load(fname)
	if err != nil {
		log.Println("unable to load", fname, "error", err)
		return strings.Join(out, ""), err
	}
	optKeys, err := schema.OptionalKeys()
	if err != nil {
		log.Println("unable to get optional keys, error", err)
		return strings.Join(out, ""), err
	}
	allKeys, err := schema.Keys()
	if err != nil {
		log.Println("unable to get keys, error", err)
		return strings.Join(out, ""), err
	}
	sectionKeys, err := schema.SectionKeys()
	if err != nil {
		log.Println("unable to get section keys, error", err)
		return strings.Join(out, ""), err
	}

	// loop over all defined sections
	var rec string
	sections, err := schema.Sections()
	if err != nil {
		log.Println("unable to get sections, error", err)
		return strings.Join(out, ""), err
	}
	for _, s := range sections {
		if skeys, ok := sectionKeys[s]; ok {
			showSection := false
			if len(skeys) != 0 {
				showSection = true
			}
			if showSection {
				out = append(out, fmt.Sprintf("<fieldset id=\"%s\">", s))
				out = append(out, fmt.Sprintf("<legend>%s</legend>", s))
			}
			for _, k := range skeys {
				required := true
				if utils.InList[string](k, optKeys) {
					required = false
				}
				rec = formEntry(c, schema.Map, k, s, required, record)
				out = append(out, rec)
			}
			if showSection {
				out = append(out, "</fieldset>")
			}
		}
	}
	// loop over the rest of section keys which did not show up in sections
	for s, skeys := range sectionKeys {
		if utils.InList[string](s, sections) {
			continue
		}
		showSection := false
		if len(skeys) != 0 {
			showSection = true
		}
		if showSection {
			out = append(out, fmt.Sprintf("<fieldset id=\"%s\">", s))
			out = append(out, fmt.Sprintf("<legend>%s</legend>", s))
		}
		for _, k := range skeys {
			required := true
			if utils.InList[string](k, optKeys) {
				required = false
			}
			rec = formEntry(c, schema.Map, k, s, required, record)
			out = append(out, rec)
		}
		if showSection {
			out = append(out, "</fieldset>")
		}
	}
	// loop over all keys which do not have sections
	var nOut []string
	for _, k := range allKeys {
		if r, ok := schema.Map[k]; ok {
			if r.Section == "" {
				required := true
				if utils.InList[string](k, optKeys) {
					required = false
				}
				rec = formEntry(c, schema.Map, k, "", required, record)
				nOut = append(nOut, rec)
			}
		}
	}
	if len(nOut) > 0 {
		out = append(out, fmt.Sprintf("<fieldset id=\"attributes\">"))
		out = append(out, "<legend>Attriburtes</legend>")
		for _, rec := range nOut {
			out = append(out, rec)
		}
		out = append(out, "</fieldset>")
	}
	form := strings.Join(out, "\n")
	tmpl := server.MakeTmpl(StaticFs, "Form")
	tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
	tmpl["Description"] = ""
	tmpl["Beamline"] = beamline
	if record != nil {
		if val, ok := (*record)["description"]; ok {
			tmpl["Description"] = val
		}
	}
	tmpl["Form"] = template.HTML(form)
	return server.TmplPage(StaticFs, "form_beamline.tmpl", tmpl), nil
}

// helper function to create form entry
func formEntry(c *gin.Context, smap map[string]beamlines.SchemaRecord, k, s string, required bool, record *map[string]any) string {
	// check if provided record has value
	var defaultValue string
	if record != nil {
		rmap := *record
		if v, ok := rmap[k]; ok {
			defaultValue = fmt.Sprintf("%v", v)
		}
		defaultValue = strings.ReplaceAll(defaultValue, "[", "")
		defaultValue = strings.ReplaceAll(defaultValue, "]", "")
	}
	tmpl := server.MakeTmpl(StaticFs, "FormEntry")
	tmpl["Key"] = k
	tmpl["Value"] = defaultValue
	tmpl["Placeholder"] = ""
	tmpl["Description"] = ""
	tmpl["Required"] = ""
	if required {
		tmpl["Required"] = "required"
	}
	if required {
		tmpl["Class"] = "hint hint-req"
	}
	tmpl["Type"] = "text"
	tmpl["Multiple"] = ""
	tmpl["Selected"] = []string{}
	if r, ok := smap[k]; ok {
		if r.Section == s {
			if r.Type == "list_str" || r.Type == "list" {
				tmpl["List"] = true
				switch values := r.Value.(type) {
				case []any:
					var vals, selected []string
					if defaultValue != "" {
						selected = append(selected, defaultValue)
					}
					tmpl["Selected"] = selected
					for _, v := range values {
						if v != defaultValue && v != "" {
							strVal := fmt.Sprintf("%v", v)
							if !utils.InList[string](strVal, selected) {
								vals = append(vals, strVal)
							}
						}
					}
					vals = utils.List2Set[string](vals)
					// for list data types, e.g. array of strings
					// we can clearly define empty default value. And if attribute is not required
					// we should use empty value first
					if !required && defaultValue == "" {
						vals = append([]string{""}, vals...)
					}
					tmpl["Value"] = vals
				default:
					tmpl["Value"] = []string{}
				}
			} else if r.Type == "bool" || r.Type == "boolean" {
				tmpl["List"] = true
				if r.Value == true {
					tmpl["Value"] = []string{"", "true", "false"}
				} else {
					tmpl["Value"] = []string{"", "false", "true"}
				}
				if defaultValue != "" {
					if defaultValue == "true" {
						tmpl["Value"] = []string{"true", "false"}
					} else {
						tmpl["Value"] = []string{"false", "true"}
					}
				}
			} else {
				if r.Value != nil {
					switch values := r.Value.(type) {
					case []any:
						tmpl["List"] = true
						var vals []string
						for _, v := range values {
							strVal := fmt.Sprintf("%v", v)
							vals = append(vals, strVal)
						}
						vals = utils.List2Set[string](vals)
						// for non list data types, e.g. array of floats
						// we cannot clearly define empty default value as it should come from
						// schema itself, and tehrefire we do not update vals with first empty value
						tmpl["Value"] = vals
					default:
						tmpl["Value"] = fmt.Sprintf("%v", r.Value)
					}
				}
			}
			if r.Multiple {
				tmpl["Multiple"] = "multiple"
			}
			desc := fmt.Sprintf("%s", r.Description)
			if desc == "" {
				desc = "Not Available"
			}
			tmpl["Description"] = desc
			tmpl["Placeholder"] = r.Placeholder
		}
	}
	return server.TmplPage(StaticFs, "form_entry.tmpl", tmpl)
}

// helper function to parser form values
func parseValue(schema *beamlines.Schema, key string, items []string) (any, error) {
	r, ok := schema.Map[key]
	if !ok {
		if srvConfig.Config.Frontend.TestMode && utils.InList(key, beamlines.SkipKeys) {
			return "", nil
		}
		msg := fmt.Sprintf("No key %s found in schema map", key)
		log.Printf("ERROR: %s", msg)
		return false, errors.New(msg)
	} else if r.Type == "list_str" {
		return items, nil
	} else if strings.HasPrefix(r.Type, "list_int") {
		// parse given values to int data type
		var vals []int
		for _, values := range items {
			for _, val := range strings.Split(values, " ") {
				v, err := strconv.Atoi(val)
				if err == nil {
					vals = append(vals, v)
				} else {
					msg := fmt.Sprintf("ERROR: unable to parse input '%v' into int data-type, %v", items, err)
					return items, errors.New(msg)
				}
			}
		}
		return vals, nil
	} else if strings.HasPrefix(r.Type, "list_float") {
		// parse given values to float data type
		var vals []float64
		for _, values := range items {
			for _, val := range strings.Split(values, " ") {
				v, err := strconv.ParseFloat(val, 64)
				if err == nil {
					vals = append(vals, v)
				} else {
					msg := fmt.Sprintf("ERROR: unable to parse input '%v' into float data-type, %v", items, err)
					return items, errors.New(msg)
				}
			}
		}
		return vals, nil
	} else if r.Type == "string" {
		return items[0], nil
	} else if r.Type == "bool" {
		v, err := strconv.ParseBool(items[0])
		if err == nil {
			return v, nil
		}
		msg := fmt.Sprintf("Unable to parse boolean value for key=%s, please come back to web form and choose either true or false", key)
		log.Printf("ERROR: %s", msg)
		return false, errors.New(msg)
	} else if strings.HasPrefix(r.Type, "int") {
		v, err := strconv.ParseInt(items[0], 10, 64)
		if err == nil {
			if r.Type == "int64" {
				return int64(v), nil
			} else if r.Type == "int32" {
				return int32(v), nil
			} else if r.Type == "int16" {
				return int16(v), nil
			} else if r.Type == "int8" {
				return int8(v), nil
			} else if r.Type == "int" {
				return int(v), nil
			}
			return v, nil
		}
		return 0, err
	} else if strings.HasPrefix(r.Type, "float") {
		v, err := strconv.ParseFloat(items[0], 64)
		if err == nil {
			if r.Type == "float32" {
				return float32(v), nil
			}
			return v, nil
		}
		return 0.0, err
	}
	msg := fmt.Sprintf("Unable to parse form value for key %s", key)
	log.Printf("ERROR: %s", msg)
	return 0, errors.New(msg)
}

// helper function to retrieve files from web user record form
func formFiles(val any) []string {
	var out []string
	switch files := val.(type) {
	case []string:
		for _, f := range files {
			if strings.Contains(f, ",") {
				for _, v := range strings.Split(f, ",") {
					v = strings.Replace(v, "\n", "", -1)
					v = strings.Replace(v, "\r", "", -1)
					v = strings.Trim(v, " ")
					if v != "" {
						out = append(out, v)
					}
				}
			} else if strings.Contains(f, "\r") {
				for _, v := range strings.Split(f, "\r") {
					v = strings.Replace(v, "\n", "", -1)
					v = strings.Replace(v, "\r", "", -1)
					v = strings.Trim(v, " ")
					if v != "" {
						out = append(out, v)
					}
				}
			} else if strings.Contains(f, "\n") {
				for _, v := range strings.Split(f, "\n") {
					v = strings.Replace(v, "\n", "", -1)
					v = strings.Replace(v, "\r", "", -1)
					v = strings.Trim(v, " ")
					if v != "" {
						out = append(out, v)
					}
				}
			} else {
				if f != "" {
					out = append(out, f)
				}
			}
		}
	}
	return out
}

// helper function to extract beamline, btr, cycle, sample_name parts from did
func extractParts(did string) (string, string, string, string) {
	var beamline, btr, cycle, sample_name string
	for _, part := range strings.Split(did, "/") {
		if strings.HasPrefix(part, "beamline=") {
			beamline = strings.Replace(part, "beamline=", "", -1)
		}
		if strings.HasPrefix(part, "btr=") {
			btr = strings.Replace(part, "btr=", "", -1)
		}
		if strings.HasPrefix(part, "cycle=") {
			cycle = strings.Replace(part, "cycle=", "", -1)
		}
		if strings.HasPrefix(part, "sample_name=") {
			sample_name = strings.Replace(part, "sample_name=", "", -1)
		}
	}
	return beamline, btr, cycle, sample_name
}

// helper function to make provenance links from a links of given dids
func makeProvenanceLinks(dids []string) []string {
	var out []string
	for _, did := range dids {
		link := fmt.Sprintf("<a href=\"/record?did=%s\" class=\"prov-link\">%s</a>", did, did)
		out = append(out, link)
	}
	return out
}
