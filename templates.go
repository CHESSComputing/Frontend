package main

// templates module
//
// Copyright (c) 2023 - Valentin Kuznetsov <vkuznet@gmail.com>
//

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
	"strconv"
	textTemplate "text/template"
	"time"
)

// TmplRecord represent template record
type TmplRecord map[string]interface{}

// GetString converts given value for provided key to string data-type
func (t TmplRecord) GetString(key string) string {
	if v, ok := t[key]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// GetInt converts given value for provided key to int data-type
func (t TmplRecord) GetInt(key string) int {
	if v, ok := t[key]; ok {
		if val, err := strconv.Atoi(fmt.Sprintf("%v", v)); err == nil {
			return val
		} else {
			log.Println("ERROR:", err)
		}
	}
	return 0
}

// GetError returns error string
func (t TmplRecord) GetError() string {
	if v, ok := t["Error"]; ok {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// GetBytes returns bytes object for given key
func (t TmplRecord) GetBytes(key string) []byte {
	if data, ok := t[key]; ok {
		return data.([]byte)
	}
	return []byte{}
}

// GetElapsedTime returns elapsed time
func (t TmplRecord) GetElapsedTime() string {
	if val, ok := t["StartTime"]; ok {
		startTime := time.Unix(val.(int64), 0)
		return time.Since(startTime).String()
	}
	return ""
}

// Templates structure
type Templates struct {
	html string
}

// Tmpl method for ServerTemplates structure
func (q Templates) Tmpl(tfile string, tmplData map[string]interface{}) string {
	if q.html != "" {
		return q.html
	}

	// get template from embed.FS
	filenames := []string{"static/templates/" + tfile}
	t := template.Must(template.New(tfile).ParseFS(StaticFs, filenames...))
	buf := new(bytes.Buffer)
	err := t.Execute(buf, tmplData)
	if err != nil {
		log.Println("ERROR: template.Tmpl", err)
		return ""
	}
	q.html = buf.String()
	return q.html
}

// Tmpl method for ServerTemplates structure
func (q Templates) TextTmpl(tfile string, tmplData map[string]interface{}) string {
	if q.html != "" {
		return q.html
	}

	// get template from embed.FS
	filenames := []string{"static/templates/" + tfile}
	t := textTemplate.Must(textTemplate.New(tfile).ParseFS(StaticFs, filenames...))
	buf := new(bytes.Buffer)
	err := t.Execute(buf, tmplData)
	if err != nil {
		log.Println("ERROR: template.Tmpl", err)
		return ""
	}
	q.html = buf.String()
	return q.html
}
