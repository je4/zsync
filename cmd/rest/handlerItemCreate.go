package main

import (
	"encoding/json/v2"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/je4/zsync/v2/pkg/zotero/model"
)

func (handlers *Handlers) makeItemCreateHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		vars := mux.Vars(r)
		// get groups object from cache
		group, err := handlers.groupFromVars(ctx, vars)
		if err != nil {
			handlers.logger.Errorf("no group: %v", err)
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("no group: %v", err))
			return
		}
		oldid, ok := vars["oldid"]
		if !ok {
			oldid = ""
		}

		var itemData model.ItemGeneric
		if err := json.UnmarshalRead(r.Body, &itemData); err != nil {
			handlers.logger.Errorf("cannot decode json: %v", err)
			respondWithError(w, http.StatusUnprocessableEntity, fmt.Sprintf("cannot decode json: %v", err))
			return
		}
		itemMeta := model.ItemMeta{}
		if handlers.client != nil && handlers.client.CurrentKey != nil {
			itemMeta.CreatedByUser = model.User{
				Id:       handlers.client.CurrentKey.UserId,
				Username: handlers.client.CurrentKey.Username,
				Links:    nil,
			}
		}
		item, err := handlers.storage.CreateItem(ctx, group.Id, &itemData, &itemMeta, oldid)
		if err != nil {
			handlers.logger.Errorf("error storing new item: %v", err)
			respondWithError(w, http.StatusUnprocessableEntity, fmt.Sprintf("error storing new item: %v", err))
			return
		}
		respondWithJSON(w, http.StatusOK, item)
	}
}
