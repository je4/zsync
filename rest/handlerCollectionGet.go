package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"net/http"
)

func (handlers *Handlers) makeCollectionGetHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		// get groups object from cache
		group, err := handlers.groupFromVars(vars)
		if err != nil {
			handlers.logger.Errorf("no group: %v", err)
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("no group: %v", err))
			return
		}
		parentKey, ok := vars["key"]
		if !ok {
			parentKey = ""
		}
		name, ok := vars["name"]
		if !ok {
			handlers.logger.Errorf("no name: %v", err)
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("no name: %v", err))
			return
		}

		coll, err := group.GetCollectionByName(name, parentKey)
		if err != nil {
			handlers.logger.Errorf("cannot get collection: %v", err)
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("cannot get collection: %v", err))
			return
		}
		if coll == nil {
			respondWithJSON(w, http.StatusNotFound, fmt.Sprintf("collection %v.%v <- %v not found", group.Id, name, parentKey))
			return
		}

		respondWithJSON(w, http.StatusOK, coll)
	}
}
