package main

import (
	"encoding/json"
	"errors"
	"github.com/bluele/gcache"
	"github.com/goph/emperror"
	"github.com/op/go-logging"
	"gitlab.fhnw.ch/hgk-dima/zotero-sync/zotero"
	"log"
	"net/http"
	"strconv"
	"time"
)

type Handlers struct {
	groups gcache.Cache
	cfg    *Config
	logger *logging.Logger
	zot    *zotero.Zotero
}

func NewHandler(zot *zotero.Zotero, cfg *Config, logger *logging.Logger) *Handlers {
	exp, err := time.ParseDuration(cfg.GroupCacheExpiration)
	if err != nil {
		log.Fatalf("error parsing expiration: %v", err)
	}

	handlers := &Handlers{
		zot:    zot,
		cfg:    cfg,
		logger: logger,
		groups:gcache.New(500).
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

func (handlers *Handlers) getGroup(groupId int64) (group *zotero.Group, err error) {
	tmp, err := handlers.groups.Get(groupId)
	if err != nil {
		group, err = handlers.zot.LoadGroupDB(groupId)
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot load group %v", groupId)
		}
		handlers.groups.Set(groupId, group)
	} else {
		var ok bool
		group, ok = tmp.(*zotero.Group)
		if !ok {
			return nil, emperror.Wrapf(errors.New("invalid type in cache"), "cannot load group %v", groupId)
		}
	}
	return
}

func (handlers *Handlers) groupFromVars(vars map[string]string) (*zotero.Group, error) {
	groupidstr, ok := vars["groupid"]
	if !ok {
		return nil, errors.New("no groupid")
	}
	groupid, err := strconv.ParseInt(groupidstr, 10, 64)
	if err != nil {
		return nil, emperror.Wrapf(err, "groupid not a number #%v", groupidstr)
	}
	return handlers.getGroup(groupid)
}
