package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/je4/zsync/v2/pkg/zotero"
	"net/http"
)

func (handlers *Handlers) makeItemCreateHandler() http.HandlerFunc {
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

		var itemData zotero.ItemGeneric
		decoder := json.NewDecoder(r.Body)
		if err := decoder.Decode(&itemData); err != nil {
			handlers.logger.Errorf("cannot decode json: %v", err)
			respondWithError(w, http.StatusUnprocessableEntity, fmt.Sprintf("cannot decode json: %v", err))
			return
		}
		itemMeta := zotero.ItemMeta{
			CreatedByUser: zotero.User{
				Id:       handlers.zot.CurrentKey.UserId,
				Username: handlers.zot.CurrentKey.Username,
				Links:    nil,
			},
		}
		item, err := group.CreateItemLocal(&itemData, &itemMeta, oldid)
		if err != nil {
			handlers.logger.Errorf("error storing new item: %v", err)
			respondWithError(w, http.StatusUnprocessableEntity, fmt.Sprintf("error storing new item: %v", err))
			return
		}
		respondWithJSON(w, http.StatusOK, item)
	}
}
