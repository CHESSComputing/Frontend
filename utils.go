package main

import (
	"io"
	"log"
	"os"
	"strings"

	"github.com/gomarkdown/markdown"
	mhtml "github.com/gomarkdown/markdown/html"
	"github.com/gomarkdown/markdown/parser"
)

// helper function to generate HTML from given markdown file
func mdToHTML(fname string) (string, error) {
	/*
		filesFS, err := fs.Sub(StaticFs, "static/markdown")
		if err != nil {
			log.Println("ERROR: unable to open static/markdown", err)
			return "", err
		}
		log.Printf("### fileFS %+v", filesFS)
		file, err := filesFS.Open(fname)
	*/
	file, err := StaticFs.Open(fname)
	if err != nil {
		log.Println("ERROR: unable to open", fname, err)
		return "", err
	}
	/*
	   file, err := os.Open(fname)
	   if err != nil {
	       log.Println("ERROR: unable to open", fname, err)
	       return "", err
	   }
	*/
	defer file.Close()
	var md []byte
	md, err = io.ReadAll(file)
	if err != nil {
		return "", err
	}

	// create markdown parser with extensions
	extensions := parser.CommonExtensions | parser.AutoHeadingIDs | parser.NoEmptyLineBeforeBlock
	p := parser.NewWithExtensions(extensions)
	doc := p.Parse(md)

	// create HTML renderer with extensions
	//     htmlFlags := mhtml.CommonFlags | mhtml.HrefTargetBlank
	htmlFlags := mhtml.CommonFlags
	opts := mhtml.RendererOptions{Flags: htmlFlags}
	renderer := mhtml.NewRenderer(opts)
	content := markdown.Render(doc, renderer)
	//     return html.EscapeString(string(content)), nil
	return string(content), nil
}

// helper function to get host domain
func domain() string {
	domain := "localhost"
	hostname, err := os.Hostname()
	if err != nil {
		log.Println("ERROR: unable to get hostname, error:", err)
	}
	if !strings.Contains(hostname, ".") {
		hostname = "localhost"
	} else {
		arr := strings.Split(hostname, ".")
		domain = strings.Join(arr[len(arr)-2:], ".")
	}
	return domain
}
