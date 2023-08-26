package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"github.com/je4/zsync/v2/pkg/zotero"
	"net/http"
)

func (handlers *Handlers) makeItemGetHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		// get groups object from cache
		group, err := handlers.groupFromVars(vars)
		if err != nil {
			handlers.logger.Errorf("no group: %v", err)
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("no group: %v", err))
			return
		}
		oldid, ok := vars["oldid"]
		if !ok {
			oldid = ""
		}
		key, ok := vars["key"]
		if !ok {
			key = ""
		}
		if key == "" && oldid == "" {
			handlers.logger.Errorf("no key or oldid")
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("no key or oldid"))
			return
		}

		var item *zotero.Item
		if key != "" {
			item, err = group.GetItemByKeyLocal(key)
		} else if oldid != "" {
			item, err = group.GetItemByOldidLocal(oldid)
		}
		if err != nil {
			handlers.logger.Errorf("cannot get item %v.%v%v: %v", group.Id, key, oldid, err)
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("cannot get item %v.%v%v: %v", group.Id, key, oldid, err))
			return
		}
		if item == nil {
			respondWithJSON(w, http.StatusNotFound, fmt.Sprintf("item %v.%v%v not found", group.Id, key, oldid))
			return
		}
		if item.Deleted {
			respondWithJSON(w, http.StatusNotFound, fmt.Sprintf("item %v.%v %v is marked as deleted", group.Id, item.Key, oldid))
			return
		}

		respondWithJSON(w, http.StatusOK, item)
	}
}
