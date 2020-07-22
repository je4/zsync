package main

import (
	"fmt"
	"github.com/gorilla/mux"
	"gitlab.fhnw.ch/hgk-dima/zotero-sync/pkg/filesystem"
	"gitlab.fhnw.ch/hgk-dima/zotero-sync/pkg/zotero"
	"net/http"
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
		folder, err := group.GetFolder()
		if err != nil {
			handlers.logger.Errorf("cannot get attachment folder")
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("cannot get attachment folder"))
			return
		}
		fs := group.Zot.GetFS()
		opts := filesystem.FilePutOptions{
			ContentType: r.Header.Get("Content-Type"),
		}
		if err := fs.FileWrite(folder, key, r.Body, -1, opts); err != nil {
			handlers.logger.Errorf("cannot write %v/%v: %v", folder, key, err)
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("cannot write %v/%v: %v", folder, key, err))
			return
		}

		item.Status = zotero.SyncStatus_Modified
		if err := item.UpdateLocal(); err != nil {
			handlers.logger.Errorf("cannot update status of  %v.%v: %v", group.Id, item.Key, err)
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("cannot update status of  %v.%v: %v", group.Id, item.Key, err))
			return
		}

		respondWithJSON(w, http.StatusOK, fmt.Sprintf("data written to %v/%v", folder, key))
	}
}
