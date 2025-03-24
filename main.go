package main

// Go implementation of FOXDEN Frontend service
//
// Copyright (c) 2023 - Valentin Kuznetsov <vkuznet@gmail.com>
//

import (
	_ "expvar" // to be used for monitoring, see https://github.com/divan/expvarmon
	"flag"
	"fmt"
	"log"
	_ "net/http/pprof" // profiler, see https://golang.org/pkg/net/http/pprof/
	"os"

	srvConfig "github.com/CHESSComputing/golib/config"
)

func main() {
	var version bool
	flag.BoolVar(&version, "version", false, "Show version")
	cfile := os.Getenv("FOXDEN_CONFIG")
	var config string
	flag.StringVar(&config, "config", cfile, "server config file, default $FOXDEN_CONFIG")
	flag.Parse()
	if version {
		fmt.Println("server version:", srvConfig.Info())
		return
	}
	if cobj, err := srvConfig.ParseConfig(config); err == nil {
		srvConfig.Config = &cobj
	} else {
		log.Fatal(fmt.Sprintf("Unable to parse config='%s'", config))
	}
	if srvConfig.Config.Frontend.WebServer.Verbose > 0 {
		log.SetFlags(log.LstdFlags | log.Lshortfile)
	}
	Server()
}
