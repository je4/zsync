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
	Service         string
	Endpoint        string
	Apikey          string
	Logfile         string
	Loglevel        string
	Libraries       []int64
	Ignorelibraries []int64
	DB              Cfg_database `toml:"database"`
}

func LoadConfig(filepath string) Config {
	var conf Config
	_, err := toml.DecodeFile(filepath, &conf)
	if err != nil {
		log.Fatalln("Error on loading config: ", err)
	}
	return conf
}
