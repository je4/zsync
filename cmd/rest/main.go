package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/je4/zsync/v2/pkg/filesystem"
	"github.com/je4/zsync/v2/pkg/zotero/client"
	"github.com/je4/zsync/v2/pkg/zotero/storage"
	"github.com/mash/go-accesslog"
	"github.com/op/go-logging"
	"github.com/rs/zerolog"
)

var _logformat = logging.MustStringFormatter(
	`%{time:2006-01-02T15:04:05.000} %{module}::%{shortfunc} > %{level:.5s} - %{message}`,
)

type alogger struct {
	handle *os.File
}

func (l alogger) Log(record accesslog.LogRecord) {
	if _, err := fmt.Fprintf(l.handle, "%s [%s] \"%s %s %s\" %d %d\n", record.Host, time.Now().Format(time.RFC3339), record.Method, record.Uri, record.Protocol, record.Status, record.Size); err != nil {
	}
}

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
	cfgfile := flag.String("c", "/etc/zotero.toml", "location of config file")
	flag.Parse()
	cfg := LoadConfig(*cfgfile)

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
	logger, lf := CreateLogger(cfg.Service, cfg.Logfile, cfg.Loglevel)
	defer lf.Close()

	fs, err := filesystem.NewS3Fs(cfg.S3.Endpoint, cfg.S3.AccessKeyId, cfg.S3.SecretAccessKey, cfg.S3.UseSSL)
	if err != nil {
		log.Fatalf("cannot connect to s3 instance: %v", err)
	}

	zlog := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()
	zotStorage := storage.NewStorage(db, cfg.NewGroupActive, &zlog)
	var zotClient *client.Client
	if cfg.Endpoint != "" {
		zotClient, err = client.NewClient(cfg.Endpoint, cfg.Apikey, &zlog)
		if err != nil {
			logger.Warningf("cannot create zotero client: %v", err)
		} else {
			logger.Infof("current key: %v", zotClient.CurrentKey)
		}
	}

	handler := NewHandler(zotStorage, zotClient, fs, &cfg, logger)

	router := mux.NewRouter()
	router.HandleFunc("/{groupid}/items", handler.makeItemCreateHandler()).Methods("POST")
	router.HandleFunc("/{groupid}/items/{key}", handler.makeItemGetHandler()).Methods("GET")
	router.HandleFunc("/{groupid}/items/{key}", handler.makeItemDeleteHandler()).Methods("DELETE")
	router.HandleFunc("/{groupid}/olditems/{oldid}", handler.makeItemCreateHandler()).Methods("POST")
	router.HandleFunc("/{groupid}/olditems/{oldid}", handler.makeItemGetHandler()).Methods("GET")
	router.HandleFunc("/{groupid}/olditems/{oldid}", handler.makeItemDeleteHandler()).Methods("DELETE")
	router.HandleFunc("/{groupid}/collections", handler.makeCollectionCreateHandler()).Methods("POST")
	router.HandleFunc("/{groupid}/items/{key}/attachment", handler.makeItemAttachmentHandler()).Methods("POST")

	var f *os.File
	if cfg.AccessLog == "" {
		f = os.Stdout
	} else {
		f, err = os.OpenFile(cfg.AccessLog, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			panic(err)
		}
	}
	defer f.Close()
	l := alogger{handle: f}
	headersOk := handlers.AllowedHeaders([]string{"Origin", "X-Requested-With", "Content-Type", "Accept", "Access-Control-Request-Method", "Authorization"})
	originsOk := handlers.AllowedOrigins([]string{"*"})
	methodsOk := handlers.AllowedMethods([]string{"GET", "HEAD", "POST", "PUT", "OPTIONS", "DELETE"})
	credentialsOk := handlers.AllowCredentials()
	ignoreOptions := handlers.IgnoreOptions()

	server := &http.Server{
		Handler: accesslog.NewLoggingHandler(handlers.CORS(
			originsOk,
			headersOk,
			methodsOk,
			credentialsOk,
			ignoreOptions,
		)(router), l),
		Addr:         cfg.Listen,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		signal.Notify(sigint, syscall.SIGTERM)

		<-sigint

		logger.Infof("shutdown requested")
		if err = server.Shutdown(context.Background()); err != nil {
			logger.Errorf("error shutting down server: %v", err)
		}
	}()

	logger.Infof("Rest Service listening on %s", cfg.Listen)
	if cfg.TLS {
		logger.Fatal(server.ListenAndServeTLS(cfg.CertChain, cfg.PrivateKey))
	} else {
		logger.Fatal(server.ListenAndServe())
	}
}
