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

type S3 struct {
	Endpoint        string `toml:"endpoint"`
	AccessKeyId     string `toml:"accessKeyId"`
	SecretAccessKey string `toml:"secretAccessKey"`
	UseSSL          bool   `toml:"useSSL"`
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
	SyncSleep            string       `toml:"syncsleep"`
	S3                   S3           `toml:"s3"`
}

func LoadConfig(filepath string) Config {
	var conf Config
	_, err := toml.DecodeFile(filepath, &conf)
	if err != nil {
		log.Fatalln("Error on loading config: ", err)
	}
	return conf
}
