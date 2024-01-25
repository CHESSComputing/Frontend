package main

// utils module
//
// Copyright (c) 2023 - Valentin Kuznetsov <vkuznet@gmail.com>
//
import (
	"log"
	"os"
	"strings"
)

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
