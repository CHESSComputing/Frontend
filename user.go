package main

import "time"

// UserInfo represents the structure returned by the user service.
type UserInfo struct {
	DN        string    `json:"DN"`
	Name      string    `json:"Name"`
	Email     string    `json:"Email"`
	Uid       string    `json:"Uid"`
	UidNumber int       `json:"UidNumber"`
	GidNumber int       `json:"GidNumber"`
	Groups    []string  `json:"Groups"`
	Btrs      []string  `json:"Btrs"`
	Beamlines []string  `json:"Beamlines"`
	Expire    time.Time `json:"Expire"`
	Foxdens   []string  `json:"Foxdens"`
}
