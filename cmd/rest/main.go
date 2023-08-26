package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/je4/zsync/v2/pkg/filesystem"
	"github.com/je4/zsync/v2/pkg/zotero"
	_ "github.com/lib/pq"
	"github.com/mash/go-accesslog"
	"github.com/op/go-logging"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var _logformat = logging.MustStringFormatter(
	`%{time:2006-01-02T15:04:05.000} %{module}::%{shortfunc} > %{level:.5s} - %{message}`,
)

type alogger struct {
	handle *os.File
}

func (l alogger) Log(record accesslog.LogRecord) {
	//log.Println(record.Host+" ["+(time.Now().Format(time.RFC3339))+"] \""+record.Method+" "+record.Uri+" "+record.Protocol+"\" "+strconv.Itoa(record.Status)+" "+strconv.FormatInt(record.Size, 10))
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

	fs, err := filesystem.NewS3Fs(cfg.S3.Endpoint, cfg.S3.AccessKeyId, cfg.S3.SecretAccessKey, cfg.S3.UseSSL)
	if err != nil {
		log.Fatalf("cannot conntct to s3 instance: %v", err)
	}

	rand.Seed(time.Now().Unix())

	zot, err := zotero.NewZotero(cfg.Endpoint, cfg.Apikey, db, fs, cfg.DB.Schema, cfg.NewGroupActive, logger, false)
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

	//	router := mux.NewRouter()

	handler := NewHandler(zot, &cfg, logger)

	router := mux.NewRouter()
	router.HandleFunc("/{groupid}/items", handler.makeItemCreateHandler()).Methods("POST")
	router.HandleFunc("/{groupid}/items/{key}", handler.makeItemGetHandler()).Methods("GET")
	router.HandleFunc("/{groupid}/items/oldid/{oldid}", handler.makeItemCreateHandler()).Methods("POST")
	router.HandleFunc("/{groupid}/items/oldid/{oldid}", handler.makeItemGetHandler()).Methods("GET")
	router.HandleFunc("/{groupid}/items/{key}/attachment", handler.makeItemAttachmentHandler()).Methods("POST")
	router.HandleFunc("/{groupid}/collections", handler.makeCollectionCreateHandler()).Methods("POST")
	router.HandleFunc("/{groupid}/collections/{name}", handler.makeCollectionGetHandler()).Methods("GET")
	router.HandleFunc("/{groupid}/collections/{key}/{name}", handler.makeCollectionGetHandler()).Methods("GET")
	router.HandleFunc("/{groupid}/items/{key}", handler.makeItemDeleteHandler()).Methods("DELETE")
	router.HandleFunc("/{groupid}/items/oldid/{oldid}", handler.makeItemDeleteHandler()).Methods("DELETE")

	var f *os.File
	if cfg.AccessLog == "" {
		f = os.Stderr
	} else {
		f, err = os.OpenFile(cfg.AccessLog, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
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
		Addr: cfg.Listen,
		// Good practice: enforce timeouts for servers you create!
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	go func() {
		sigint := make(chan os.Signal, 1)

		// interrupt signal sent from terminal
		signal.Notify(sigint, os.Interrupt)
		//		signal.Notify(sigint, syscall.SIGINT)
		signal.Notify(sigint, syscall.SIGTERM)

		<-sigint

		// We received an interrupt signal, shut down.
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
