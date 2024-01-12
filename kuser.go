package main

// module to handle kerberos access
//
// Copyright (c) 2023 - Valentin Kuznetsov <vkuznet@gmail.com>
//

import (
	"log"

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
