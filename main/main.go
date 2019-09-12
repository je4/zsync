package main

import (
	"database/sql"
	"flag"
	_ "github.com/lib/pq"
	"github.com/op/go-logging"
	"gitlab.fhnw.ch/hgk-dima/zotero-sync/zotero"
	"log"
	"os"
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

	groupVersions, err := zot.GetUserGroupVersions(key)
	if err != nil {
		logger.Errorf("cannot get group versions: %v", err)
		return
	}
	logger.Infof("group versions: %v", groupVersions)

	groupIds := []int64{}
	for groupId, version := range *groupVersions {
		ignore := false
		if len(cfg.Libraries) > 0 {
			ignore = true
			for _, id := range cfg.Libraries {
				if id == groupId {
					ignore = false
					break
				}
			}
		}
		if ignore {
			logger.Infof("library %v not in postive list", groupId)
			continue
		}
		// check whether library is configured as ignore
		for _, ignoreId := range cfg.Ignorelibraries {
			if ignoreId == groupId {
				ignore = true
				break
			}
		}
		if ignore {
			logger.Infof("library %v in negative list", groupId)
			continue
		}

		groupIds = append(groupIds, groupId)
		group, err := zot.LoadGroupDB(groupId)
		if err != nil {
			logger.Errorf("cannot load group %v: %v", groupId, err)
			return
		}

		_, err = group.SyncCollections()
		if err != nil {
			logger.Errorf("cannot sync collections of group %v: %v", groupId, err)
			return
		}
		_, err = group.SyncItems()
		if err != nil {
			logger.Errorf("cannot sync items of group %v: %v", groupId, err)
			return
		}
		_, err = group.SyncTags()
		if err != nil {
			logger.Errorf("cannot sync tags of group %v: %v", groupId, err)
			return
		}

		delcoll, delitem, deltag, err := group.GetDeleted(group.Version)
		if err != nil {
			logger.Errorf("cannot get deletions of group %v: %v", groupId, err)
			return
		}
		if delitem != nil {
			for _, itemKey := range *delitem {
				if err := zot.DeleteItemDB(itemKey); err != nil {
					logger.Errorf("cannot delete item %s: %v", itemKey, err)
					return
				}
			}
		}
		if delcoll != nil {
			for _, colKey := range *delcoll {
				if err := zot.DeleteCollectionDB(colKey); err != nil {
					logger.Errorf("cannot delete item %s: %v", colKey, err)
					return
				}
			}
		}
		if deltag != nil {
			for _, tag := range *deltag {
				if err := zot.DeleteCollectionDB(tag); err != nil {
					logger.Errorf("cannot delete tag %s: %v", tag, err)
					return
				}
			}
		}

		// store new group data if necessary
		logger.Infof("group %v[%v <-> %v]", groupId, group.Version, version)
		// check whether version is newer online...
		if group.Version < version || group.Deleted {
			if group.Version == 0 {
				if err := zot.CreateEmptyGroupDB(groupId); err != nil {
					logger.Errorf("cannot create empty group: %v", err)
					return
				}
			}
			newGroup, err := zot.GetGroup(groupId)
			if err != nil {
				logger.Errorf("cannot get group %v: %v", groupId, err)
				return
			}
			logger.Infof("group %v[%v]", groupId, version)
			if err := newGroup.UpdateDB(); err != nil {
				logger.Errorf("cannot update group %v: %v", groupId, err)
				return
			}
		}
	}
	if err := zot.DeleteUnknownGroups(groupIds); err != nil {
		logger.Errorf("cannot delete unknown groups: %v", err)
	}
}
