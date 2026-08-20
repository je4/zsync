package main

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/je4/zsync/v2/pkg/zotero/model"
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
		var collectionData model.CollectionData
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&collectionData); err != nil {
			handlers.logger.Errorf("cannot decode json: %v", err)
			respondWithError(w, http.StatusUnprocessableEntity, fmt.Sprintf("cannot decode json: %v", err))
			return
		}
		coll, err := handlers.storage.CreateCollection(group.Id, &collectionData)
		if err != nil {
			handlers.logger.Errorf("error storing new collection: %v", err)
			respondWithError(w, http.StatusUnprocessableEntity, fmt.Sprintf("error storing new collection: %v", err))
			return
		}
		respondWithJSON(w, http.StatusOK, coll)
	}
}
