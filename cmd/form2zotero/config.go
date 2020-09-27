/*
Copyright 2020 Center for Digital Matter HGK FHNW, Basel.
Copyright 2020 info-age GmbH, Basel.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS-IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package main

import (
	"fmt"
	"github.com/BurntSushi/toml"
	"log"
)

type database struct {
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
	Logfile           string   `toml:"logfile"`
	Loglevel          string   `toml:"loglevel"`
	FormDB            database `toml:SourceDB`
	ZoteroDB          database `toml:ZoteroDB`
	Endpoint          string
	Apikey            string
	S3                S3 `toml:"s3"`
	AttachementFolder string
}

func LoadConfig(filepath string) Config {
	var conf Config
	m, err := toml.DecodeFile(filepath, &conf)
	if err != nil {
		log.Fatalln("Error on loading config: ", err)
	}
	fmt.Sprintf("%v", m)
	return conf
}
