package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/je4/zsync/v2/pkg/filesystem"
	"github.com/je4/zsync/v2/pkg/zotero/storage"
	"github.com/je4/zsync/v2/pkg/zotero/sync"
	"github.com/op/go-logging"
	"github.com/rs/zerolog"
)

var _logformat = logging.MustStringFormatter(
	`%{time:2006-01-02T15:04:05.000} %{module}::%{shortfunc} [%{shortfile}] > %{level:.5s} - %{message}`,
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

type ZotField struct {
	Field     string `json:"field"`
	Localized string `json:"localized"`
}

func backup(cfg *Config, db *pgxpool.Pool, fs filesystem.FileSystem, logger *logging.Logger) {
	backupFs, err := filesystem.NewLocalFs(cfg.Backup.Path, logger)
	if err != nil {
		logger.Panicf("not a git repo: %v", cfg.Backup.Path)
	}

	zlog := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()
	zotStorage := storage.NewStorage(db, false, &zlog)
	syncer := sync.NewSyncer(nil, zotStorage, fs, &zlog)

	grps, err := zotStorage.LoadGroups()
	if err != nil {
		logger.Errorf("cannot load groups: %v", err)
		return
	}

	for _, grp := range grps {
		doBackup := true
		if len(cfg.Synconly) > 0 {
			doBackup = false
			for _, x := range cfg.Synconly {
				if x == grp.Id {
					doBackup = true
					break
				}
			}
		}
		if !doBackup {
			logger.Infof("ignoring group %v [%v]", grp.Data.Name, grp.Id)
			continue
		}
		if err := syncer.BackupLocal(grp, backupFs); err != nil {
			logger.Errorf("cannot backup group #%v: %v", grp.Id, err)
		}
	}
}

func main() {
	cfgfile := flag.String("c", "/etc/zoterosync.toml", "location of config file")
	flag.Parse()
	cfg := LoadConfig(*cfgfile)

	// get database connection handle
	db, err := pgxpool.New(context.Background(), cfg.DB.DSN)
	if err != nil {
		log.Fatalf("error opening database: %v", err)
	}
	defer db.Close()

	// Validate DSN connection:
	err = db.Ping(context.Background())
	if err != nil {
		log.Fatalf("error pinging database: %v", err)
	}
	logger, lf := CreateLogger(cfg.Service, cfg.Logfile, cfg.Loglevel)
	defer lf.Close()

	fs, err := filesystem.NewS3Fs(cfg.S3.Endpoint, cfg.S3.AccessKeyId, cfg.S3.SecretAccessKey, cfg.S3.UseSSL)
	if err != nil {
		log.Fatalf("cannot connect to s3 instance: %v", err)
	}

	backup(&cfg, db, fs, logger)
}
