package main

// Go implementation of FOXDEN Frontend service
//
// Copyright (c) 2023 - Valentin Kuznetsov <vkuznet@gmail.com>
//

import (
	_ "expvar"         // to be used for monitoring, see https://github.com/divan/expvarmon
	_ "net/http/pprof" // profiler, see https://golang.org/pkg/net/http/pprof/

	srvConfig "github.com/CHESSComputing/golib/config"
)

func main() {
	srvConfig.Init()
	Server()
}
