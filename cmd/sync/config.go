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

type Cfg_gitlab struct {
	Token   string `toml:"token"`
	Project string `toml:"project"`
	Url     string `toml:"url"`
	Active  bool   `toml:"active"`
}

type S3 struct {
	Endpoint        string `toml:"endpoint"`
	AccessKeyId     string `toml:"accessKeyId"`
	SecretAccessKey string `toml:"secretAccessKey"`
	UseSSL          bool   `toml:"useSSL"`
}

type Config struct {
	Service              string
	Synconly             []int64
	ClearBeforeSync      []int64
	Endpoint             string
	Apikey               string
	Logfile              string
	Loglevel             string
	AccessLog            string
	NewGroupActive       bool `toml:"newgroupactive"`
	Attachmentfolder     string
	DB                   Cfg_database `toml:"database"`
	GroupCacheExpiration string       `toml:"groupcacheexpiration"`
	Gitlab               Cfg_gitlab   `tomal:"gitlab"`
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
