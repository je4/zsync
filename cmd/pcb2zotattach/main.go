package main

import (
	"database/sql"
	"flag"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/je4/zsync/v2/pkg/filesystem"
	"github.com/je4/zsync/v2/pkg/zotero"
	_ "github.com/lib/pq"
	"github.com/op/go-logging"
	"log"
	"math/rand"
	"os"
	"regexp"
	"time"
)

type logger struct {
	handle *os.File
}

var zoterogroup int64 = 1624911
var mediacollection int64 = 44

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

// var linkRegexp = regexp.MustCompile("https://ba14ns21403.fhnw.ch/video/open/(.+)$")
// var linkRegexp = regexp.MustCompile("file://ba14ns21403.fhnw.ch/nfsdata/www/html/video/open/(.+)$")
var linkRegexp = regexp.MustCompile("https://ba14ns21403-sec1.fhnw.ch/mediasrv/([^/]+)/([^/]+)$")

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// get location of config file
	cfgfile := flag.String("cfg", "/etc/mediasrv2.toml", "location of config file")
	flag.Parse()
	config := LoadConfig(*cfgfile)

	// create logger instance
	logger, lf := CreateLogger("memostream", config.Logfile, config.Loglevel)
	defer lf.Close()

	// get database connection handle
	mediaserverDB, err := sql.Open(config.MediaserverDB.ServerType, config.MediaserverDB.DSN)
	if err != nil {
		panic(err.Error())
	}
	defer mediaserverDB.Close()

	// Open doesn't open a connection. Validate DSN data:
	err = mediaserverDB.Ping()
	if err != nil {
		panic(err.Error())
	}

	// get database connection handle
	zoteroDB, err := sql.Open(config.ZoteroDB.ServerType, config.ZoteroDB.DSN)
	if err != nil {
		panic(err.Error())
	}
	defer zoteroDB.Close()

	// Open doesn't open a connection. Validate DSN data:
	err = zoteroDB.Ping()
	if err != nil {
		panic(err.Error())
	}

	fs, err := filesystem.NewS3Fs(config.S3.Endpoint, config.S3.AccessKeyId, config.S3.SecretAccessKey, config.S3.UseSSL)
	if err != nil {
		log.Fatalf("cannot conntct to s3 instance: %v", err)
	}

	rand.Seed(time.Now().Unix())

	zot, err := zotero.NewZotero(config.Endpoint, config.Apikey, zoteroDB, fs, config.ZoteroDB.Schema, false, logger, false)
	if err != nil {
		logger.Errorf("cannot create zotero instance: %v", err)
		return
	}

	grp, err := zot.LoadGroupLocal(zoterogroup)
	if err != nil {
		fmt.Errorf("cannot load group #%v - %v", zoterogroup, err)
		return
	}

	sqlstr := `SELECT "key" FROM s3.item_type_hier WHERE "library" = $1 AND "type" = $2`
	rows, err := zoteroDB.Query(sqlstr, zoterogroup, "attachment")
	if err != nil {
		logger.Errorf("cannot execute query %s - %v", sqlstr, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			logger.Error("cannot scan key")
			return
		}
		item, err := grp.GetItemByKeyLocal(key)
		if err != nil {
			logger.Errorf("cannot load item #%v.%v", zoterogroup, key)
			return
		}
		url := item.Data.Url
		logger.Infof(url)
		matches := linkRegexp.FindStringSubmatch(url)
		if matches != nil {
			//			file := fmt.Sprintf("file://ba14ns21403.fhnw.ch/nfsdata/www/html/video/open/%s", matches[1])
			//			msgroup := fmt.Sprintf("zotero_%v", zoterogroup)
			//			mssig := fmt.Sprintf("%v", slug.Make(matches[1]))
			mediaserver := fmt.Sprintf("https://ba14ns21403-sec1.fhnw.ch/mediasrv/%v/%v/master", matches[1], matches[2])
			logger.Infof("--> %s", mediaserver)
			item.Data.Url = mediaserver

			/*
				sqlstr := fmt.Sprintf("INSERT INTO master (collectionid, signature, urn) VALUES( ?, ?, ?)")
				var params []interface{} = []interface{}{mediacollection, mssig, file}

				if _, err := mediaserverDB.Exec(sqlstr, params...); err != nil {
					logger.Errorf("cannot execute insert query %v", params...)
				}
			*/

			if err := item.UpdateLocal(); err != nil {
				logger.Errorf("cannot update #%v.%v", zoterogroup, key)
			}
		}
	}
}
