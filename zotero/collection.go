package zotero

import (
	"encoding/json"
	"fmt"
	"github.com/goph/emperror"
)

type CollectionLibrary struct {
	Type  string      `json:"type"`
	Id    int64       `json:"id"`
	Name  string      `json:"name"`
	Links interface{} `json:"links"`
}

type Collection struct {
	Key     string                 `json:"key"`
	Version int64                  `json:"version"`
	Library CollectionLibrary      `json:"library,omitempty"`
	Links   interface{}            `json:"links,omitempty"`
	Meta    interface{}            `json:"meta,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
	group   *Group                 `json:"-"`
	Status  SyncStatus             `json:"-"`
}

func (collection *Collection) UpdateCollectionDB() error {
	collection.group.zot.logger.Infof("Updating Collection [#%s]", collection.Key)
	data, err := json.Marshal(collection.Data)
	if err != nil {
		return emperror.Wrapf(err, "cannot marshall data %v", collection.Data)
	}
	sqlstr := fmt.Sprintf("UPDATE %s.collections SET version=$1, sync=$2, data=$3 WHERE key=$4", collection.group.zot.dbSchema)
	params := []interface{}{
		collection.Version,
		SyncStatusString[collection.Status],
		data,
		collection.Key,
	}
	_, err = collection.group.zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}
