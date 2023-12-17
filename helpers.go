package main

import (
	"fmt"
	"html/template"
	"log"
	"strings"

	beamlines "github.com/CHESSComputing/golib/beamlines"
	mongo "github.com/CHESSComputing/golib/mongo"
	server "github.com/CHESSComputing/golib/server"
	utils "github.com/CHESSComputing/golib/utils"
	"github.com/gin-gonic/gin"
)

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
func genForm(c *gin.Context, fname string, record *mongo.Record) (string, error) {
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
	tmpl["Beamline"] = beamline
	tmpl["Form"] = template.HTML(form)
	return server.TmplPage(StaticFs, "form_beamline.tmpl", tmpl), nil
}

// helper function to create form entry
func formEntry(c *gin.Context, smap map[string]beamlines.SchemaRecord, k, s, required string, record *mongo.Record) string {
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
