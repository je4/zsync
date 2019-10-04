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
	"os/signal"
	"syscall"
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

func sync(cfg *Config, db *sql.DB, logger *logging.Logger) {
	zot, err := zotero.NewZotero(cfg.Endpoint, cfg.Apikey, db, cfg.DB.Schema, cfg.Attachmentfolder, cfg.NewGroupActive, logger)
	if err != nil {
		logger.Errorf("cannot create zotero instance: %v", err)
		return
	}

	logger.Infof("current key: %v", zot.CurrentKey)

	groupVersions, err := zot.GetUserGroupVersions(zot.CurrentKey)
	if err != nil {
		logger.Errorf("cannot get group versions: %v", err)
		return
	}
	logger.Infof("group versions: %v", groupVersions)

	groupIds := []int64{}
	for groupId, version := range *groupVersions {
		groupIds = append(groupIds, groupId)
		group, err := zot.LoadGroupLocal(groupId)
		if err != nil {
			logger.Errorf("cannot load group %v: %v", groupId, err)
			return
		}
		if !group.Active {
			logger.Infof("ignoring inactive group #%v", group.Id)
			continue
		}

		if err := group.Sync(); err != nil {
			logger.Errorf("cannot sync group #%v: %v", group.Id, err)
			continue
		}

		// store new group data if necessary
		logger.Infof("group %v[%v <-> %v]", groupId, group.Version, version)
		// check whether version is newer online...
		if group.Version < version ||
			group.Deleted  ||
			group.Modified {
			newGroup, err := zot.GetGroupCloud(groupId)
			if err != nil {
				logger.Errorf("cannot get group %v: %v", groupId, err)
				return
			}
			newGroup.CollectionVersion = group.CollectionVersion
			newGroup.ItemVersion = group.ItemVersion
			newGroup.TagVersion = group.TagVersion
			newGroup.Deleted = group.Deleted

			logger.Infof("group %v[%v]", groupId, version)
			if err := newGroup.UpdateLocal(); err != nil {
				logger.Errorf("cannot update group %v: %v", groupId, err)
				return
			}
		}
	}
	if err := zot.DeleteUnknownGroupsLocal(groupIds); err != nil {
		logger.Errorf("cannot delete unknown groups: %v", err)
	}

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

	sleep, err := time.ParseDuration(cfg.SyncSleep)
	if err != nil {
		logger.Fatalf("error parsing syncsleep: %v", err)
	}
	c1 := make(chan string, 1)

	go func() {
		sigint := make(chan os.Signal, 1)

		// interrupt signal sent from terminal
		signal.Notify(sigint, os.Interrupt)
		// sigterm signal sent from kubernetes
		signal.Notify(sigint, syscall.SIGTERM)

		<-sigint

		// We received an interrupt signal, shut down.
		logger.Infof("shutdown requested")
		c1 <- "end please"

	}()
		for {
			sync(&cfg, db, logger)
			logger.Infof("sleeping %v", cfg.SyncSleep)
			select {
			case <-c1:
				return
			case <-time.After(sleep):
				logger.Infof("end of sleep")
			}
		}

}
