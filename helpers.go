package main

// helpers module
//
// Copyright (c) 2023 - Valentin Kuznetsov <vkuznet@gmail.com>
//
import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"sort"
	"strconv"
	"strings"

	beamlines "github.com/CHESSComputing/golib/beamlines"
	srvConfig "github.com/CHESSComputing/golib/config"
	server "github.com/CHESSComputing/golib/server"
	utils "github.com/CHESSComputing/golib/utils"
	"github.com/gin-gonic/gin"
)

//
// helper functions
//

// helper function to provide error page
func handleError(c *gin.Context, status int, msg string, err error) {
	page := server.ErrorPage(StaticFs, msg, err)
	c.Data(status, "text/html; charset=utf-8", []byte(header()+page+footer()))
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

// helper function to prepare HTML page for given services records
func records2html(user string, records []map[string]any) string {
	var out []string
	for _, rec := range records {
		tmpl := server.MakeTmpl(StaticFs, "Record")
		tmpl["User"] = user
		tmpl["Id"] = recValue(rec, "_id")
		tmpl["Did"] = recValue(rec, "did")
		tmpl["Cycle"] = recValue(rec, "Cycle")
		tmpl["Beamline"] = recValue(rec, "Beamline")
		tmpl["Btr"] = recValue(rec, "BTR")
		tmpl["Sample"] = recValue(rec, "SampleName")
		tmpl["Schema"] = recValue(rec, "Schema")
		tmpl["Base"] = srvConfig.Config.Frontend.WebServer.Base
		tmpl["Record"] = rec
		tmpl["RecordTable"] = reprRecord(rec, "table")
		tmpl["RecordDescription"] = reprRecord(rec, "description")
		tmpl["RecordJSON"] = reprRecord(rec, "json")
		tmpl["Description"] = recValue(rec, "Description")
		content := server.TmplPage(StaticFs, "record.tmpl", tmpl)
		out = append(out, content)
	}
	return strings.Join(out, "\n")
}

// SchemaDetails represents individual FOXDEN schema details dictionary
type SchemaDetails struct {
	Schema       string            `json:"schema"`
	Units        map[string]string `json:"units"`
	Descriptions map[string]string `json:"descriptions"`
}

// SchemaManager holds SchemaDetails list for all FOXDEN schemas
type SchemaManager struct {
	Records []SchemaDetails
}

func (s *SchemaManager) initManager() []SchemaDetails {
	var records []SchemaDetails
	if s == nil {
		s = &SchemaManager{}
		// fetch all schema details from upstream MetaData server
		rurl := fmt.Sprintf("%s/meta", srvConfig.Config.Services.MetaDataURL)
		if resp, err := _httpReadRequest.Get(rurl); err == nil {
			defer resp.Body.Close()
			if data, err := io.ReadAll(resp.Body); err == nil {
				if err := json.Unmarshal(data, &records); err == nil {
					s.Records = records
				}
			}
		}
	} else {
		records = s.Records
	}
	return records
}

// helper function to find schema units map for given schema name
func (s *SchemaManager) findUnits(sname string) map[string]string {
	records := s.initManager()
	for _, rec := range records {
		if rec.Schema != sname {
			continue
		}
		return rec.Units
	}
	empty := make(map[string]string)
	return empty
}

// helper function to find schema units map for given schema name
func (s *SchemaManager) findDescriptions(sname string) map[string]string {
	records := s.initManager()
	for _, rec := range records {
		if rec.Schema != sname {
			continue
		}
		return rec.Descriptions
	}
	empty := make(map[string]string)
	return empty
}

var _schemaManager *SchemaManager

// helper function to represent record
func reprRecord(rec map[string]any, format string) string {
	sname := recValue(rec, "Schema")
	smap := _schemaManager.findUnits(sname)
	dmap := _schemaManager.findDescriptions(sname)
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
		for key, _ := range rec {
			if desc, ok := dmap[key]; ok {
				out = fmt.Sprintf("%s\n%s: %v", out, utils.PaddedKey(key, maxLen), desc)
			} else {
				out = fmt.Sprintf("%s\n%s: Not Available", out, utils.PaddedKey(key, maxLen))
			}
		}
		return out
	}
	for key, val := range rec {
		if unit, ok := smap[key]; ok {
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
func pagination(c *gin.Context, query string, nres, startIdx, limit int) string {
	tmpl := server.MakeTmpl(StaticFs, "Search")
	url := fmt.Sprintf("/search?query=%s", query)
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
	page := server.TmplPage(StaticFs, "pagination.tmpl", tmpl)
	return fmt.Sprintf("%s<br>", page)
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
				if utils.InList[string](k, optKeys) {
					rec = formEntry(c, schema.Map, k, s, "", record)
				} else {
					rec = formEntry(c, schema.Map, k, s, "required", record)
				}
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
			if utils.InList[string](k, optKeys) {
				rec = formEntry(c, schema.Map, k, s, "required", record)
			} else {
				rec = formEntry(c, schema.Map, k, s, "", record)
			}
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
				if utils.InList[string](k, optKeys) {
					rec = formEntry(c, schema.Map, k, "", "", record)
				} else {
					rec = formEntry(c, schema.Map, k, "", "required", record)
				}
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
	tmpl["Beamline"] = beamline
	tmpl["Form"] = template.HTML(form)
	return server.TmplPage(StaticFs, "form_beamline.tmpl", tmpl), nil
}

// helper function to create form entry
func formEntry(c *gin.Context, smap map[string]beamlines.SchemaRecord, k, s, required string, record *map[string]any) string {
	// check if provided record has value
	var defaultValue string
	if record != nil {
		rmap := *record
		if v, ok := rmap[k]; ok {
			defaultValue = fmt.Sprintf("%v", v)
		}
		defaultValue = strings.Replace(defaultValue, "[", "", -1)
		defaultValue = strings.Replace(defaultValue, "]", "", -1)
	}
	tmpl := server.MakeTmpl(StaticFs, "FormEntry")
	tmpl["Key"] = k
	tmpl["Value"] = defaultValue
	tmpl["Placeholder"] = ""
	tmpl["Description"] = ""
	tmpl["Required"] = required
	if required != "" {
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
						var vals []string
						if defaultValue != "" {
							vals = append(vals, defaultValue)
						}
						for _, v := range values {
							vstr := fmt.Sprintf("%v", v)
							for _, vvv := range strings.Split(vstr, ",") {
								vals = append(vals, vvv)
							}
						}
						var out []string
						vstr := strings.Join(vals, ",")
						for _, vvv := range strings.Split(vstr, ",") {
							out = append(out, vvv)
						}
						vals = utils.List2Set[string](out)
						tmpl["Value"] = strings.Join(vals, ",")
					default:
						tmpl["Value"] = fmt.Sprintf("%v", r.Value)
						if defaultValue != "" {
							tmpl["Value"] = fmt.Sprintf("%v", defaultValue)
						}
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
				v, err := strconv.ParseFloat(val, 32)
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
