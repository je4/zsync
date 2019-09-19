package main

import (
	"database/sql"
	"flag"
	_ "github.com/lib/pq"
	"github.com/op/go-logging"
	"gitlab.fhnw.ch/hgk-dima/zotero-sync/zotero"
	"log"
	"math/rand"
	"os"
	"time"
)

var _logformat = logging.MustStringFormatter(
	`%{time:2006-01-02T15:04:05.000} %{module}::%{shortfunc} > %{level:.5s} - %{message}`,
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

type ZotField struct {
	Field     string `json:"field"`
	Localized string `json:"localized"`
}

func main() {

	cfgfile := flag.String("c", "/etc/zoterosync.toml", "location of config file")
	flag.Parse()
	cfg := LoadConfig(*cfgfile)

	// get database connection handle
	db, err := sql.Open(cfg.DB.ServerType, cfg.DB.DSN)
	if err != nil {
		log.Fatalf("error opening database: %v", err)
	}
	defer db.Close()

	// Open doesn't open a connection. Validate DSN data:
	err = db.Ping()
	if err != nil {
		log.Fatalf("error pinging database: %v", err)
	}
	logger, lf := CreateLogger(cfg.Service, cfg.Logfile, cfg.Loglevel)
	defer lf.Close()

	rand.Seed(time.Now().Unix())

	zot, err := zotero.NewZotero(cfg.Endpoint, cfg.Apikey, db, cfg.DB.Schema, cfg.Attachmentfolder, logger)
	if err != nil {
		logger.Errorf("cannot create zotero instance: %v", err)
		return
	}

	key, err := zot.GetCurrentKey()
	if err != nil {
		logger.Errorf("cannot get current key: %v", err)
		return
	}
	logger.Infof("current key: %v", key)

//	r := mux.NewRouter()

	zot.PrintTypeStructs()




}
