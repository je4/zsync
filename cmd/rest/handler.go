package main

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"emperror.dev/errors"
	"encoding/json"
	"github.com/bluele/gcache"
	"github.com/je4/zsync/v2/pkg/filesystem"
	"github.com/je4/zsync/v2/pkg/zotero/client"
	"github.com/je4/zsync/v2/pkg/zotero/model"
	"github.com/je4/zsync/v2/pkg/zotero/storage"
	"github.com/op/go-logging"
)

type Handlers struct {
	groups  gcache.Cache
	cfg     *Config
	logger  *logging.Logger
	storage *storage.Storage
	client  *client.Client
	fs      filesystem.FileSystem
}

func NewHandler(storage *storage.Storage, client *client.Client, fs filesystem.FileSystem, cfg *Config, logger *logging.Logger) *Handlers {
	exp, err := time.ParseDuration(cfg.GroupCacheExpiration)
	if err != nil {
		log.Fatalf("error parsing expiration: %v", err)
	}

	handlers := &Handlers{
		storage: storage,
		client:  client,
		fs:      fs,
		cfg:     cfg,
		logger:  logger,
		groups: gcache.New(500).
			ARC().Expiration(exp).
			Build(),
	}
	return handlers
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}

func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
	response, _ := json.Marshal(payload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

func (handlers *Handlers) getGroup(groupId int64) (group *model.Group, err error) {
	tmp, err := handlers.groups.Get(groupId)
	if err != nil {
		group, err = handlers.storage.LoadGroup(groupId)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot load group %v", groupId)
		}
		handlers.groups.Set(groupId, group)
	} else {
		var ok bool
		group, ok = tmp.(*model.Group)
		if !ok {
			return nil, errors.Wrapf(errors.New("invalid type in cache"), "cannot load group %v", groupId)
		}
	}
	return
}

func (handlers *Handlers) groupFromVars(vars map[string]string) (*model.Group, error) {
	groupidstr, ok := vars["groupid"]
	if !ok {
		return nil, errors.New("no groupid")
	}
	groupid, err := strconv.ParseInt(groupidstr, 10, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "groupid not a number #%v", groupidstr)
	}
	return handlers.getGroup(groupid)
}
