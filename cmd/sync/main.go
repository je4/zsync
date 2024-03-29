package main

import (
	"database/sql"
	"flag"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/je4/zsync/v2/pkg/filesystem"
	"github.com/je4/zsync/v2/pkg/zotero"
	_ "github.com/lib/pq"
	"github.com/op/go-logging"
	"github.com/rs/zerolog"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"
)

var _logformat = logging.MustStringFormatter(
	`%{time:2006-01-02T15:04:05.000} %{module}::%{shortfunc} > %{level:.5s} - %{message}`,
)

//
//  XBYUCYUR 2f1180d8-6582-4143-8bba-e82c9f724023
//

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

func sync(cfg *Config, db *sql.DB, fs filesystem.FileSystem, logger zLogger.ZLogger) {

	var err error
	/*
		nodes, _, err := gl.Repositories.ListTree(glproject.ID, nil)
		for _, node := range nodes {
			logger.Info().Msgf("[%v] %v - %v", node.ID, node.Name, node.Path)
		}
	*/

	zot, err := zotero.NewZotero(cfg.Endpoint, cfg.Apikey, db, fs, cfg.DB.Schema, cfg.NewGroupActive, logger, false)
	if err != nil {
		logger.Error().Msgf("cannot create zotero instance: %v", err)
		return
	}

	logger.Info().Msgf("current key: %v", zot.CurrentKey)

	groupVersions, err := zot.GetUserGroupVersions(zot.CurrentKey)
	if err != nil {
		logger.Error().Msgf("cannot get group versions: %v", err)
		return
	}
	logger.Info().Msgf("group versions: %v", groupVersions)

	groupIds := []int64{}
	for groupId, version := range *groupVersions {
		/*
			if groupId != 1510019 {
				continue
			}
		*/
		groupIds = append(groupIds, groupId)

		if len(cfg.Synconly) > 0 {
			found := false
			for _, sg := range cfg.Synconly {
				if sg == groupId {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		group, err := zot.LoadGroupLocal(groupId)
		if err != nil {
			logger.Error().Msgf("cannot load group local %v: %v", groupId, err)
			return
		}
		if !group.Active {
			logger.Info().Msgf("ignoring inactive group #%v", group.Id)
			continue
		}

		for _, gid := range cfg.ClearBeforeSync {
			if gid == group.Id {
				if err := group.ClearLocal(); err != nil {
					logger.Error().Msgf("cannot clear group %v: %v", groupId, err)
					return
				}
				break
			}
		}

		if err := group.Sync(); err != nil {
			logger.Error().Msgf("cannot sync group #%v: %v", group.Id, err)
			continue
		}

		// store new group data if necessary
		logger.Info().Msgf("group %v[%v <-> %v]", groupId, group.Version, version)
		// check whether version is newer online...
		if group.Version < version ||
			group.Deleted ||
			group.IsModified {
			newGroup, err := zot.GetGroupCloud(groupId)
			if err != nil {
				logger.Error().Msgf("cannot get group %v: %v", groupId, err)
				return
			}
			newGroup.CollectionVersion = group.CollectionVersion
			newGroup.ItemVersion = group.ItemVersion
			newGroup.TagVersion = group.TagVersion
			newGroup.Deleted = group.Deleted

			logger.Info().Msgf("group %v[%v]", groupId, version)
			if err := newGroup.UpdateLocal(); err != nil {
				logger.Error().Msgf("cannot update group %v: %v", groupId, err)
				return
			}
		}
	}
	if err := zot.DeleteUnknownGroupsLocal(groupIds); err != nil {
		logger.Error().Msgf("cannot delete unknown groups: %v", err)
	}

}

func main() {

	cfgfile := flag.String("c", "", "location of config file")
	//loop := flag.Bool("loop", false, "endless sync")
	clear := flag.Bool("clear", false, "clear all data of group")
	groupid := flag.Int64("group", 0, "id of zotero group to sync")

	flag.Parse()

	var configFile = *cfgfile
	if configFile == "" {
		if _, err := os.Stat("zoterosync.toml"); err == nil {
			configFile = "zoterosync.toml"
		} else {
			ex, err := os.Executable()
			if err != nil {
				panic(err)
			}
			exPath := filepath.Dir(ex)
			if _, err := os.Stat(filepath.Join(exPath, "zoterosync.toml")); err == nil {
				configFile = filepath.Join(exPath, "zoterosync.toml")
			}
		}
	}

	cfg := LoadConfig(configFile)

	// if local group is selected, build groups
	if *groupid > 0 {
		cfg.Synconly = []int64{*groupid}
		cfg.ClearBeforeSync = []int64{}
		if *clear {
			cfg.ClearBeforeSync = append(cfg.ClearBeforeSync, *groupid)
		}
	}

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

	var out io.Writer = os.Stdout
	if cfg.Logfile != "" {
		fp, err := os.OpenFile(cfg.Logfile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			log.Fatalf("cannot open logfile %s: %v", cfg.Logfile, err)
		}
		defer fp.Close()
		out = fp
	}

	output := zerolog.ConsoleWriter{Out: out, TimeFormat: time.RFC3339}
	_logger := zerolog.New(output).With().Timestamp().Logger()
	_logger.Level(zLogger.LogLevel(cfg.Loglevel))
	var logger zLogger.ZLogger = &_logger

	fs, err := filesystem.NewS3Fs(cfg.S3.Endpoint, cfg.S3.AccessKeyId, cfg.S3.SecretAccessKey, cfg.S3.UseSSL)
	if err != nil {
		log.Fatalf("cannot conntct to s3 instance: %v", err)
	}

	sync(&cfg, db, fs, logger)

}
