package main

import (
	"github.com/BurntSushi/toml"
	"log"
)

type Cfg_database struct {
	ServerType string
	DSN        string
	ConnMax    int `toml:"connection_max"`
	Schema     string
}

type Config struct {
	Service              string
	Listen               string
	TLS                  bool
	CertChain            string
	PrivateKey           string
	Endpoint             string
	Apikey               string
	Logfile              string
	Loglevel             string
	AccessLog            string
	NewGroupActive       bool `toml:"newgroupactive"`
	Attachmentfolder     string
	DB                   Cfg_database `toml:"database"`
	GroupCacheExpiration string       `toml:"groupcacheexpiration"`
}

func LoadConfig(filepath string) Config {
	var conf Config
	_, err := toml.DecodeFile(filepath, &conf)
	if err != nil {
		log.Fatalln("Error on loading config: ", err)
	}
	return conf
}
