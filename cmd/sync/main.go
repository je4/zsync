package main

import (
	"context"
	"flag"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/je4/utils/v2/pkg/zLogger"
	"github.com/je4/zsync/v2/pkg/filesystem"
	"github.com/je4/zsync/v2/pkg/zotero/client"
	"github.com/je4/zsync/v2/pkg/zotero/storage"
	"github.com/je4/zsync/v2/pkg/zotero/sync"
	"github.com/rs/zerolog"
)

type ZotField struct {
	Field     string `json:"field"`
	Localized string `json:"localized"`
}

func doSync(cfg *Config, db *pgxpool.Pool, fs filesystem.FileSystem, logger zLogger.ZLogger) {
	zotClient, err := client.NewClient(cfg.Endpoint, cfg.Apikey, logger)
	if err != nil {
		logger.Error().Msgf("cannot create zotero client: %v", err)
		return
	}

	zotStorage := storage.NewStorage(db, cfg.DB.Schema, cfg.NewGroupActive, logger)
	syncer := sync.NewSyncer(zotClient, zotStorage, fs, logger)

	logger.Info().Msgf("current key: %v", zotClient.CurrentKey)

	groupVersions, err := zotClient.GetUserGroupVersions(zotClient.CurrentKey)
	if err != nil {
		logger.Error().Msgf("cannot get group versions: %v", err)
		return
	}
	logger.Info().Msgf("group versions: %v", groupVersions)

	groupIds := []int64{}
	for groupId, version := range *groupVersions {
		groupIds = append(groupIds, groupId)

		if len(cfg.Synconly) > 0 {
			found := slices.Contains(cfg.Synconly, groupId)
			if !found {
				continue
			}
		}
		group, err := zotStorage.LoadGroup(groupId)
		if err != nil {
			logger.Error().Msgf("cannot load group local %v: %v", groupId, err)
			return
		}
		if !group.Active {
			logger.Info().Msgf("ignoring inactive group #%v", group.Id)
			continue
		}

		if slices.Contains(cfg.ClearBeforeSync, group.Id) {
			if err := zotStorage.ClearGroup(groupId); err != nil {
				logger.Error().Msgf("cannot clear group %v: %v", groupId, err)
				return
			}
			group.CollectionVersion = 0
			group.ItemVersion = 0
			group.Version = 0
		}

		if err := syncer.SyncGroup(group); err != nil {
			logger.Error().Msgf("cannot sync group #%v: %v", group.Id, err)
			continue
		}

		// store new group data if necessary
		logger.Info().Msgf("group %v[%v <-> %v]", groupId, group.Version, version)
		// check whether version is newer online...
		if group.Version < version ||
			group.Deleted ||
			group.IsModified {
			newGroup, err := zotClient.GetGroup(groupId)
			if err != nil {
				logger.Error().Msgf("cannot get group %v: %v", groupId, err)
				return
			}
			if newGroup != nil {
				newGroup.CollectionVersion = group.CollectionVersion
				newGroup.ItemVersion = group.ItemVersion
				newGroup.TagVersion = group.TagVersion
				newGroup.Deleted = group.Deleted
				newGroup.Active = group.Active
				newGroup.Direction = group.Direction
				newGroup.SyncTags = group.SyncTags

				logger.Info().Msgf("group %v[%v]", groupId, version)
				if err := zotStorage.UpdateGroup(newGroup); err != nil {
					logger.Error().Msgf("cannot update group %v: %v", groupId, err)
					return
				}
			}
		}
	}
}

func main() {
	cfgfile := flag.String("c", "", "location of config file")
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
	db, err := pgxpool.New(context.Background(), cfg.DB.DSN)
	if err != nil {
		log.Fatalf("error opening database: %v", err)
	}
	defer db.Close()

	// Validate DSN data:
	err = db.Ping(context.Background())
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
		log.Fatalf("cannot connect to s3 instance: %v", err)
	}

	doSync(&cfg, db, fs, logger)
}
