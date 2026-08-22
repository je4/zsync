package main

import (
	"log"

	"github.com/BurntSushi/toml"
	"github.com/je4/utils/v2/pkg/config"
)

type Cfg_database struct {
	ServerType string
	DSN        config.EnvString
	ConnMax    int `toml:"connection_max"`
}

type Cfg_gitlab struct {
	Token   config.EnvString `toml:"token"`
	Project string           `toml:"project"`
	Url     string           `toml:"url"`
	Active  bool             `toml:"active"`
}

type S3 struct {
	Endpoint        string           `toml:"endpoint"`
	AccessKeyId     config.EnvString `toml:"accessKeyId"`
	SecretAccessKey config.EnvString `toml:"secretAccessKey"`
	UseSSL          bool             `toml:"useSSL"`
}

type Config struct {
	Service              string
	Synconly             []int64
	ClearBeforeSync      []int64
	Endpoint             string
	Apikey               config.EnvString
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
