package main

import (
	"log"
	"net/http"
	"strconv"
	"time"

	"encoding/json/v2"

	"emperror.dev/errors"
	"github.com/je4/zsync/v2/pkg/filesystem"
	"github.com/je4/zsync/v2/pkg/zotero/client"
	"github.com/je4/zsync/v2/pkg/zotero/model"
	"github.com/je4/zsync/v2/pkg/zotero/storage"
	"github.com/maypok86/otter/v2"
	"github.com/op/go-logging"
)

type Handlers struct {
	groups  *otter.Cache[int64, *model.Group]
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

	cache, err := otter.New(&otter.Options[int64, *model.Group]{
		MaximumSize:      500,
		ExpiryCalculator: otter.ExpiryWriting[int64, *model.Group](exp),
	})
	if err != nil {
		log.Fatalf("error initializing otter cache: %v", err)
	}

	handlers := &Handlers{
		storage: storage,
		client:  client,
		fs:      fs,
		cfg:     cfg,
		logger:  logger,
		groups:  cache,
	}
	return handlers
}

func respondWithError(w http.ResponseWriter, code int, message string) {
	respondWithJSON(w, code, map[string]string{"error": message})
}

func respondWithJSON(w http.ResponseWriter, code int, payload any) {
	response, _ := json.Marshal(payload)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	w.Write(response)
}

func (handlers *Handlers) getGroup(groupId int64) (*model.Group, error) {
	group, ok := handlers.groups.GetIfPresent(groupId)
	if !ok {
		var err error
		group, err = handlers.storage.LoadGroup(groupId)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot load group %v", groupId)
		}
		handlers.groups.Set(groupId, group)
	}
	return group, nil
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
