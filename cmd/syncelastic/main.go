package main

import (
	"database/sql"
	"flag"
	_ "github.com/go-sql-driver/mysql"
	"github.com/je4/zsync/pkg/filesystem"
	"github.com/je4/zsync/pkg/zotero"
	"github.com/je4/zsync/pkg/zotmedia"
	"github.com/op/go-logging"
	"log"
	"os"
	"time"
)

type logger struct {
	handle *os.File
}

var _logformat = logging.MustStringFormatter(
	`%{time:2006-01-02T15:04:05.000} %{shortfunc} > %{level:.5s} - %{message}`,
)

func CreateLogger(module string, logfile string, loglevel string) (log *logging.Logger, lf *os.File) {
	log = logging.MustGetLogger(module)
	var err error
	if logfile != "" {
		lf, err = os.OpenFile(logfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Errorf("Cannot open logfile %v: %v", logfile, err)
		}
		//defer lf.Close()

	} else {
		lf = os.Stderr
	}
	backend := logging.NewLogBackend(lf, "", 0)
	backendLeveled := logging.AddModuleLevel(backend)
	backendLeveled.SetLevel(logging.GetLevel(loglevel), "")

	logging.SetFormatter(_logformat)
	logging.SetBackend(backendLeveled)

	return
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// get location of config file
	cfgfile := flag.String("cfg", "/etc/syncelastic.toml", "location of config file")
	flag.Parse()
	config := LoadConfig(*cfgfile)

	// create logger instance
	logger, lf := CreateLogger("memostream", config.Logfile, config.Loglevel)
	defer lf.Close()

	// get database connection handle
	zoteroDB, err := sql.Open(config.ZoteroDB.ServerType, config.ZoteroDB.DSN)
	if err != nil {
		log.Fatalf("error opening database: %v", err)
	}
	defer zoteroDB.Close()

	// Open doesn't open a connection. Validate DSN data:
	err = zoteroDB.Ping()
	if err != nil {
		log.Fatalf("error pinging database: %v", err)
	}

	mediaDB, err := sql.Open(config.MediaDB.ServerType, config.MediaDB.DSN)
	if err != nil {
		log.Fatalf("error opening database: %v", err)
	}
	defer mediaDB.Close()

	// Open doesn't open a connection. Validate DSN data:
	err = mediaDB.Ping()
	if err != nil {
		log.Fatalf("error pinging database: %v", err)
	}

	fs, err := filesystem.NewS3Fs(config.S3.Endpoint, config.S3.AccessKeyId, config.S3.SecretAccessKey, config.S3.UseSSL)
	if err != nil {
		log.Fatalf("cannot conntct to s3 instance: %v", err)
	}

	zot, err := zotero.NewZotero(config.Endpoint, config.Apikey, zoteroDB, fs, config.ZoteroDB.Schema, false, logger, false)
	if err != nil {
		logger.Errorf("cannot create zotero instance: %v", err)
		return
	}

	logger.Infof("Zotero key: #%v %v", zot.CurrentKey.UserId, zot.CurrentKey.Username)

	ms, err := zotmedia.NewMediaserverMySQL(config.Mediaserverbase, mediaDB, config.MediaDB.Schema, logger)
	if err != nil {
		logger.Panicf("cannot create mediaserver: %v", err)
	}
	logger.Infof("%v", ms)
	/*
		cfg := elasticsearch.Config{
			Addresses: config.ElasticEndpoint,
			// ...
		}
		es, err := elasticsearch.NewClient(cfg)
		if err != nil {
			logger.Panicf("cannot create elasticsearch client: %v", err)
		}

		res, err := es.Info()
		if err != nil {
			logger.Panicf("cannot get elasticsearch info: %v", err)
		}
		defer res.Body.Close()
		// Check response status
		if res.IsError() {
			logger.Panicf("Error: %s", res.String())
		}
		// Deserialize the response into a map.
		var r map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&r); err != nil {
			logger.Panicf("Error parsing the response body: %s", err)
		}
		// Print client and server version numbers.
		logger.Infof("Client: %s", elasticsearch.Version)
		logger.Infof("Server: %s", r["version"].(map[string]interface{})["number"])

	*/

	groups, err := zot.LoadGroupsLocal()
	if err != nil {
		logger.Panicf("cannot load groups: %v", err)
	}

	logger.Infof("%v", groups)
	after := time.Unix(0, 0)
	for _, group := range groups {
		if len(config.Synconly) > 0 {
			ok := false
			for _, sog := range config.Synconly {
				if sog == group.Id {
					ok = true
					break
				}
			}
			if !ok {
				continue
			}
		}
		group.IterateItemsAllLocal(&after, func(item *zotero.Item) error {
			// ignore all notes and attachments
			if item.Data.ParentItem != "" {
				return nil
			}
			//medias := item.GetMedia(ms)
			acls := item.GetACL()
			logger.Infof("%v", acls)
			return nil
		})
	}

	//esapi.Create()
}
