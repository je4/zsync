package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"

	_ "github.com/go-sql-driver/mysql"
	"github.com/je4/zsync/v2/pkg/zotero/storage"
	_ "github.com/lib/pq"
	"github.com/op/go-logging"
	"github.com/rs/zerolog"
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
	cfgfile := flag.String("cfg", "/etc/pcb2zotattach.toml", "location of config file")
	flag.Parse()
	config := LoadConfig(*cfgfile)

	// create logger instance
	logger, lf := CreateLogger("pcb2zotattach", config.Logfile, config.Loglevel)
	defer lf.Close()

	linkRegexp := regexp.MustCompile("^https://ba14ns21403.fhnw.ch/video/(open|intern)/([^/]+)$")

	// get database connection handle
	mediaserverDB, err := sql.Open(config.MediaserverDB.ServerType, config.MediaserverDB.DSN)
	if err != nil {
		log.Fatalf("error opening database: %v", err)
	}
	defer mediaserverDB.Close()

	// Open doesn't open a connection. Validate DSN data:
	err = mediaserverDB.Ping()
	if err != nil {
		log.Fatalf("error pinging database: %v", err)
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

	zlog := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()
	zotStorage := storage.NewStorage(zoteroDB, config.ZoteroDB.Schema, false, &zlog)

	sqlstr := fmt.Sprintf(`SELECT "key" FROM %s.item_type_hier WHERE "library" = $1 AND "type" = $2`, config.ZoteroDB.Schema)
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
		item, err := zotStorage.GetItemByKey(zoterogroup, key)
		if err != nil {
			logger.Errorf("cannot load item #%v.%v", zoterogroup, key)
			return
		}
		if item == nil {
			continue
		}
		url := item.Data.Url
		logger.Infof(url)
		matches := linkRegexp.FindStringSubmatch(url)
		if matches != nil {
			mediaserver := fmt.Sprintf("https://ba14ns21403-sec1.fhnw.ch/mediasrv/%v/%v/master", matches[1], matches[2])
			logger.Infof("--> %s", mediaserver)
			item.Data.Url = mediaserver

			if err := zotStorage.UpdateItem(zoterogroup, item); err != nil {
				logger.Errorf("cannot update #%v.%v", zoterogroup, key)
			}
		}
	}
}
