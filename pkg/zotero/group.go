package zotero

import (
	"encoding/json"
	"fmt"
	"github.com/goph/emperror"
	"github.com/je4/zsync/pkg/filesystem"
	"gopkg.in/resty.v1"
	"path"
	"strconv"
	"time"
)

type GroupMeta struct {
	Created      time.Time `json:"created"`
	LastModified time.Time `json:"lastModified"`
	NumItems     int64     `json:"numItems"`
}

type GroupData struct {
	Id             int64   `json:"id"`
	Version        int64   `json:"version"`
	Name           string  `json:"name"`
	Owner          int64   `json:owner`
	Type           string  `json:"type"`
	Description    string  `json:"description"`
	Url            string  `json:"url"`
	HasImage       int64   `json:"hasImage"`
	LibraryEditing string  `json:libraryEditing`
	LibraryReading string  `json:libraryReading`
	FileEditing    string  `json:fileEditing`
	Admins         []int64 `json:"admins"`
}

type Group struct {
	Id                int64               `json:"id"`
	Version           int64               `json:"version"`
	Links             interface{}         `json:"links,omitempty"`
	Meta              GroupMeta           `json:"meta"`
	Data              GroupData           `json:"data"`
	Deleted           bool                `json:"-"`
	Active            bool                `json:"-"`
	syncTags          bool                `json:"-"` // sync tags?
	direction         SyncDirection       `json:"-"`
	Zot               *Zotero             `json:"-"`
	ItemVersion       int64               `json:"-"`
	CollectionVersion int64               `json:"-"`
	TagVersion        int64               `json:"-"`
	IsModified        bool                `json:"-"`
	Gitlab            *time.Time          `json:"-"`
	Folder            string              `json:"-"`
	meta              map[string][]string `json:"-"`
}

type GroupGitlab struct {
	Id                int64     `json:"id"`
	Data              GroupData `json:"data,omitempty"`
	CollectionVersion int64     `json:"collectionversion"`
	ItemVersion       int64     `json:"itemversion"`
	TagVersion        int64     `json:tagversion`
}

func (group *Group) Init() {
	group.meta = Text2Metadata(group.Data.Description)
}

func (group *Group) BackupLocal(backupFs filesystem.FileSystem) error {
	now := time.Now()

	folder := fmt.Sprintf("%v", group.Id)

	if err := group.IterateCollectionsAllLocal(group.Gitlab, func(coll *Collection) error {
		group.Zot.logger.Infof("collection #%v.%v - %v", coll.Group.Id, coll.Key, coll.Data.Name)
		if err := coll.Backup(backupFs); err != nil {
			return emperror.Wrapf(err, "cannot backup collection #%v.%v", coll.Group.Id, coll.Key)
		}
		return nil
	}); err != nil {
		return emperror.Wrap(err, "cannot iterate collections")
	}
	sqlstr := fmt.Sprintf("UPDATE %s.collections SET gitlab=TO_TIMESTAMP($1, 'YYYY-MM-DD HH24:MI:SS') "+
		"WHERE library=$2 AND (TO_TIMESTAMP($3, 'YYYY-MM-DD HH24:MI:SS') > gitlab OR gitlab IS NULL)", group.Zot.dbSchema)
	params := []interface{}{
		now.Format("2006-01-02 15:04:05"),
		group.Id,
		now.Format("2006-01-02 15:04:05"),
	}
	if group.Gitlab != nil {
		sqlstr += " AND (gitlab >= TO_TIMESTAMP($4, 'YYYY-MM-DD HH24:MI:SS') OR gitlab IS NULL)"
		params = append(params, group.Gitlab.Format("2006-01-02 15:04:05"))
	}
	_, err := group.Zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %v - %v", sqlstr, params)
	}

	if err := group.IterateItemsAllLocal(group.Gitlab, func(item *Item) error {
		group.Zot.logger.Infof("item #%v.%v - %v", item.Group.Id, item.Key, item.Data.Title)
		if err := item.Backup(backupFs); err != nil {
			return emperror.Wrapf(err, "cannot backup item #%v.%v", item.Group.Id, item.Key)
		}
		return nil
	}); err != nil {
		return emperror.Wrap(err, "cannot iterate items")
	}

	sqlstr = fmt.Sprintf("UPDATE %s.items SET gitlab=TO_TIMESTAMP($1, 'YYYY-MM-DD HH24:MI:SS') "+
		"WHERE library=$2 AND (gitlab <= TO_TIMESTAMP($3, 'YYYY-MM-DD HH24:MI:SS') OR gitlab IS NULL)", group.Zot.dbSchema)
	params = []interface{}{
		now.Format("2006-01-02 15:04:05"),
		group.Id,
		now.Format("2006-01-02 15:04:05"),
	}
	if group.Gitlab != nil {
		sqlstr += " AND (gitlab >= TO_TIMESTAMP($4, 'YYYY-MM-DD HH24:MI:SS') OR gitlab IS NULL)"
		params = append(params, group.Gitlab.Format("2006-01-02 15:04:05"))
	}
	_, err = group.Zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %v - %v", sqlstr, params)
	}

	storeGrp := group.Gitlab == nil
	if group.Gitlab != nil {
		// modification local disk-version is older
		if group.Gitlab.Before(group.Meta.LastModified) {
			storeGrp = true
		}
	}
	if storeGrp {
		data, err := json.MarshalIndent(group.Data, "", "  ")
		if err != nil {
			group.Zot.logger.Errorf("cannot marshal Group data of #%v", group.Id)
		} else {
			filename := path.Clean(fmt.Sprintf("%v.json", group.Id))
			if err := backupFs.FilePut(folder, filename, data, filesystem.FilePutOptions{}); err != nil {
				group.Zot.logger.Errorf("cannot write Group data of #%v to file %v", group.Id, filename)
			}
		}
	}

	sqlstr = fmt.Sprintf("UPDATE %s.groups SET gitlab=TO_TIMESTAMP($1, 'YYYY-MM-DD HH24:MI:SS') WHERE id=$2", group.Zot.dbSchema)
	if _, err := group.Zot.db.Exec(sqlstr, now, group.Id); err != nil {
		return emperror.Wrapf(err, "cannot update timestamp for group #%v", group.Id)
	}

	return nil
}

func (group *Group) GetLibrary() *Library {
	return &Library{
		Type:  group.Data.Type,
		Id:    group.Id,
		Name:  group.Data.Name,
		Links: group.Links,
	}
}

func (group *Group) UpdateLocal() error {
	sqlstr := fmt.Sprintf("UPDATE %s.groups SET version=$1, created=$2, modified=$3, data=$4, deleted=$5,"+
		" itemversion=$6, collectionversion=$7, tagversion=$8"+
		" WHERE id=$9", group.Zot.dbSchema)
	data, err := json.MarshalIndent(group.Data, "", "  ")
	if err != nil {
		return emperror.Wrapf(err, "cannot marshal Group data")
	}

	params := []interface{}{
		group.Version,
		group.Meta.Created,
		group.Meta.LastModified,
		data,
		group.Deleted,
		group.ItemVersion,
		group.CollectionVersion,
		group.TagVersion,
		group.Id,
	}
	_, err = group.Zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}

	return nil
}

func (group *Group) GetFolder() (string, error) {
	if group.Folder == "" {
		// create bucket
		bucket := fmt.Sprintf("zotero-%v", group.Id)
		found, err := group.Zot.fs.FolderExists(bucket)
		if err != nil {
			return "", emperror.Wrap(err, "cannot check bucket existence")
		}
		if !found {
			if err := group.Zot.fs.FolderCreate(bucket, filesystem.FolderCreateOptions{ObjectLocking: true}); err != nil {
				return "", emperror.Wrapf(err, "cannot create bucket %s", bucket)
			}
		}
		group.Folder = bucket
	}
	return group.Folder, nil
}

func (group *Group) SyncDeleted() (int64, error) {
	if !group.CanDownload() {
		return 0, nil
	}
	endpoint := fmt.Sprintf("/groups/%v/deleted", group.Id)
	group.Zot.logger.Infof("rest call: %s [%v]", endpoint, group.Version)
	call := group.Zot.client.R().
		SetHeader("Accept", "application/json").
		SetQueryParam("since", strconv.FormatInt(group.Version, 10))
	var resp *resty.Response
	var err error
	for {
		resp, err = call.Get(endpoint)
		if err != nil {
			return 0, emperror.Wrapf(err, "cannot get deleted objects from %s", endpoint)
		}
		if !group.Zot.CheckRetry(resp.Header()) {
			break
		}
	}
	limv := resp.RawResponse.Header.Get("Last-Modified-Version")
	lastModifiedVersion, err := strconv.ParseInt(limv, 10, 64)
	if err != nil {
		return 0, emperror.Wrapf(err, "cannot convert 'Last-Modified-Version' - %v", limv)
	}
	rawBody := resp.Body()
	deletions := Deletions{}
	if err := json.Unmarshal(rawBody, &deletions); err != nil {
		return 0, emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	for _, itemKey := range deletions.Items {
		if err := group.TryDeleteItemLocal(itemKey, lastModifiedVersion); err != nil {
			return 0, emperror.Wrapf(err, "cannot delete item %v", itemKey)
		}
	}
	for _, collectionKey := range deletions.Collections {
		if err := group.TryDeleteCollectionLocal(collectionKey, lastModifiedVersion); err != nil {
			return 0, emperror.Wrapf(err, "cannot delete collection %v", collectionKey)
		}
	}
	for _, tagName := range deletions.Tags {
		if err := group.DeleteTagLocal(tagName); err != nil {
			return 0, emperror.Wrapf(err, "cannot delete tag %v", tagName)
		}
	}

	group.Zot.CheckBackoff(resp.Header())

	return int64(len(deletions.Tags) + len(deletions.Items) + len(deletions.Collections)), nil
}

func (group *Group) Sync() (err error) {
	// no sync at all
	if group.direction == SyncDirection_None {
		return nil
	}

	_, itemVersion, err := group.UploadItems()
	if err != nil {
		return emperror.Wrapf(err, "cannot sync items of Group %v", group.Id)
	}

	_, collectionVersion, err := group.SyncCollections()
	if err != nil {
		return emperror.Wrapf(err, "cannot sync collections of Group %v", group.Id)
	}
	_, itemVersion, err = group.DownloadItems()
	if err != nil {
		return emperror.Wrapf(err, "cannot sync items of Group %v", group.Id)
	}
	_, tagVersion, err := group.SyncTags()
	if err != nil {
		return emperror.Wrapf(err, "cannot sync tags of Group %v", group.Id)
	}

	_, err = group.SyncDeleted()
	if err != nil {
		return emperror.Wrapf(err, "cannot sync tags of Group %v", group.Id)
	}

	// change to new version if everything was ok
	if collectionVersion > group.CollectionVersion {
		group.CollectionVersion = collectionVersion
		group.IsModified = true
	}
	if itemVersion > group.ItemVersion {
		group.ItemVersion = itemVersion
		group.IsModified = true
	}
	if tagVersion > group.TagVersion {
		group.TagVersion = tagVersion
		group.IsModified = true
	}

	return
}

func (group *Group) CanUpload() bool {
	return group.direction == SyncDirection_BothCloud ||
		group.direction == SyncDirection_BothLocal ||
		group.direction == SyncDirection_BothManual ||
		group.direction == SyncDirection_ToCloud
}

func (group *Group) CanDownload() bool {
	return group.direction == SyncDirection_BothCloud ||
		group.direction == SyncDirection_BothLocal ||
		group.direction == SyncDirection_BothManual ||
		group.direction == SyncDirection_ToLocal
}
