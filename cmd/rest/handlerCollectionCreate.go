package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"gitlab.fhnw.ch/hgk-dima/zotero-sync/pkg/zotero"
	"net/http"
)

func (handlers *Handlers) makeCollectionCreateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		// get groups object from cache
		group, err := handlers.groupFromVars(vars)
		if err != nil {
			handlers.logger.Errorf("no group: %v", err)
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("no group: %v", err))
			return
		}
		var collectionData zotero.CollectionData
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&collectionData); err != nil {
			handlers.logger.Errorf("cannot decode json: %v", err)
			respondWithError(w, http.StatusUnprocessableEntity, fmt.Sprintf("cannot decode json: %v", err))
			return
		}
		coll, err := group.CreateCollectionLocal(&collectionData)
		if err != nil {
			handlers.logger.Errorf("error storing new item: %v", err)
			respondWithError(w, http.StatusUnprocessableEntity, fmt.Sprintf("error storing new item: %v", err))
			return
		}
		respondWithJSON(w, http.StatusOK, coll)
	}
}

