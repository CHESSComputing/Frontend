package main

// module to handle kerberos access
//
// Copyright (c) 2023 - Valentin Kuznetsov <vkuznet@gmail.com>
//

import (
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	srvConfig "github.com/CHESSComputing/golib/config"
	"gopkg.in/jcmturner/gokrb5.v7/client"
	"gopkg.in/jcmturner/gokrb5.v7/config"
	"gopkg.in/jcmturner/gokrb5.v7/credentials"
)

// https://github.com/jcmturner/gokrb5/issues/7
func kuserFromCache(cacheFile string) (*credentials.Credentials, error) {
	cfg, err := config.Load(srvConfig.Config.Kerberos.Krb5Conf)
	ccache, err := credentials.LoadCCache(cacheFile)
	client, err := client.NewClientFromCCache(ccache, cfg)
	err = client.Login()
	if err != nil {
		return nil, err
	}
	return client.Credentials, nil

}

// helper function to perform kerberos authentication
func kuser(user, password string) (*credentials.Credentials, error) {
	cfg, err := config.Load(srvConfig.Config.Kerberos.Krb5Conf)
	if err != nil {
		log.Printf("reading krb5.conf failes, error %v\n", err)
		return nil, err
	}
	client := client.NewClientWithPassword(user, srvConfig.Config.Kerberos.Realm, password, cfg, client.DisablePAFXFAST(true))
	err = client.Login()
	if err != nil {
		log.Printf("client login fails, error %v\n", err)
		return nil, err
	}
	return client.Credentials, nil
}

// helper function to check user credentials for POST requests
func getUserCredentials(r *http.Request) (*credentials.Credentials, error) {
	var msg string
	// user didn't use web interface, we switch to POST form
	name := r.FormValue("name")
	ticket := r.FormValue("ticket")
	tmpFile, err := ioutil.TempFile("/tmp", name)
	if err != nil {
		msg = fmt.Sprintf("Unable to create tempfile: %v", err)
		log.Printf("ERROR: %s", msg)
		return nil, errors.New(msg)
	}
	defer os.Remove(tmpFile.Name())
	_, err = tmpFile.Write([]byte(ticket))
	if err != nil {
		msg = "unable to write kerberos ticket"
		log.Printf("ERROR: %s", msg)
		return nil, errors.New(msg)
	}
	err = tmpFile.Close()
	creds, err := kuserFromCache(tmpFile.Name())
	if err != nil {
		msg = "wrong user credentials"
		log.Printf("ERROR: %s", msg)
		return nil, errors.New(msg)
	}
	if creds == nil {
		msg = "unable to obtain user credentials"
		log.Printf("ERROR: %s", msg)
		return nil, errors.New(msg)
	}
	return creds, nil
}
