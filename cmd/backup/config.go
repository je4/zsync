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

type Backup struct {
	Path  string
	Name  string
	Email string
}

type Config struct {
	Service              string
	Logfile              string
	Loglevel             string
	AccessLog            string
	Synconly             []int64
	Backup               Backup
	DB                   Cfg_database `toml:"database"`
	GroupCacheExpiration string       `toml:"groupcacheexpiration"`
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
