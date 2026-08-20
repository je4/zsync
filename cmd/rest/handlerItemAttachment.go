package main

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/je4/zsync/v2/pkg/filesystem"
	"github.com/je4/zsync/v2/pkg/zotero/model"
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
		items, err := handlers.storage.GetItems(group.Id, []string{key})
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
		bucket := fmt.Sprintf("zotero-%v", group.Id)
		if handlers.fs != nil {
			found, err := handlers.fs.FolderExists(bucket)
			if err != nil {
				handlers.logger.Errorf("cannot check bucket existence")
				respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("cannot check bucket existence: %v", err))
				return
			}
			if !found {
				if err := handlers.fs.FolderCreate(bucket, filesystem.FolderCreateOptions{}); err != nil {
					handlers.logger.Errorf("cannot create bucket %s", bucket)
					respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("cannot create bucket %s: %v", bucket, err))
					return
				}
			}
			opts := filesystem.FilePutOptions{
				ContentType: r.Header.Get("Content-Type"),
			}
			if err := handlers.fs.FileWrite(bucket, key, r.Body, -1, opts); err != nil {
				handlers.logger.Errorf("cannot write %v/%v: %v", bucket, key, err)
				respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("cannot write %v/%v: %v", bucket, key, err))
				return
			}
		}

		item.Status = model.SyncStatus_Modified
		if err := handlers.storage.UpdateItem(group.Id, &item); err != nil {
			handlers.logger.Errorf("cannot update status of %v.%v: %v", group.Id, item.Key, err)
			respondWithError(w, http.StatusInternalServerError, fmt.Sprintf("cannot update status of %v.%v: %v", group.Id, item.Key, err))
			return
		}

		respondWithJSON(w, http.StatusOK, fmt.Sprintf("data written to %v/%v", bucket, key))
	}
}
