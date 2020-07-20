package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"gitlab.fhnw.ch/hgk-dima/zotero-sync/pkg/zotero"
	"io"
	"net/http"
	"os"
)

func (handlers *Handlers) makeItemAttachmentHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		// get groups object from cache
		group, err := handlers.groupFromVars(vars)
		if err != nil {
			handlers.logger.Errorf("no group: %v", err)
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("no group: %v", err))
			return
		}
		key, ok := vars["key"]
		if !ok {
			handlers.logger.Errorf("no key in url")
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("no key in url"))
			return
		}
		items, err := group.GetItemsLocal([]string{key})
		if err != nil {
			handlers.logger.Errorf("could not load item #%v.%v", group.Id, key)
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("could not load item #%v.%v", group.Id, key))
			return
		}
		if len(*items) == 0 {
			handlers.logger.Errorf("could not find item #%v.%v", group.Id, key)
			respondWithError(w, http.StatusNotFound, fmt.Sprintf("could not find item #%v.%v", group.Id, key))
			return
		}
		item := (*items)[0]
		if item.Data.ItemType != "attachment" {
			handlers.logger.Errorf("item %v.%v is not an attachment", group.Id, key)
			respondWithError(w, http.StatusForbidden, fmt.Sprintf("item %v.%v is not an attachment", group.Id, key))
			return
		}
		folder, err := group.GetAttachmentFolder()
		if err != nil {
			handlers.logger.Errorf("cannot get attachment folder")
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("cannot get attachment folder"))
			return
		}
		filename := fmt.Sprintf("%s/%s", folder, key)
		f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0666)
		if err != nil {
			handlers.logger.Errorf("cannot create %v: %v", filename, err)
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("cannot create %v: %v", filename, err))
			return
		}
		defer f.Close()
		num, err := io.Copy(f, r.Body)
		if err != nil {
			handlers.logger.Errorf("cannot write %v: %v", filename, err)
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("cannot write %v: %v", filename, err))
			return
		}

		item.Status = zotero.SyncStatus_Modified
		if err := item.UpdateLocal(); err != nil {
			handlers.logger.Errorf("cannot update status of  %v.%v: %v", group.Id, item.Key, err)
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("cannot update status of  %v.%v: %v", group.Id, item.Key, err))
			return
		}

		respondWithJSON(w, http.StatusOK, fmt.Sprintf("%v byte written to %v", num, filename))
	}
}
