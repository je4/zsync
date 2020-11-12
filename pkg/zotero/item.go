package zotero

import (
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/goph/emperror"
	"github.com/je4/zsync/pkg/filesystem"
	"gopkg.in/resty.v1"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type StringOrBool string

func (sb *StringOrBool) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		s = ""
	}
	var h StringOrBool
	h = StringOrBool(s)
	sb = &h
	return nil
}

type ItemMeta struct {
	CreatedByUser  User         `json:"createdByUser"`
	CreatorSummary string       `json:"creatorSummary,omitempty"`
	ParsedDate     StringOrBool `json:"parsedDate,omitempty"`
	NumChildren    int64        `json:"numChildren,omitempty"`
}

type Item struct {
	Key     string      `json:"key"`
	Version int64       `json:"version"`
	Library Library     `json:"library,omitempty"`
	Links   interface{} `json:"links,omitempty"`
	Meta    ItemMeta    `json:"meta,omitempty"`
	Data    ItemGeneric `json:"data,omitempty"`
	Group   *Group      `json:"-"`
	Trashed bool        `json:"-"`
	Deleted bool        `json:"-"`
	Status  SyncStatus  `json:"-"`
	MD5     string      `json:"-"`
	OldId   string      `json:"-"`
	Gitlab  *time.Time  `json:"-"`
}

type ItemTag struct {
	Tag  string `json:"tag"`
	Type int64  `json:"type,omitempty"`
}

type ItemDataBase struct {
	Key          string                      `json:"key,omitempty"`
	Version      int64                       `json:"version"`
	ItemType     string                      `json:"itemType"`
	Tags         []ItemTag                   `json:"tags"`
	Relations    map[string]ZoteroStringList `json:"relations"`
	ParentItem   Parent                      `json:"parentItem,omitempty"`
	Collections  []string                    `json:"collections"`
	DateAdded    string                      `json:"dateAdded,omitempty"`
	DateModified string                      `json:"dateModified,omitempty"`
	Creators     []ItemDataPerson            `json:"creators,omitempty"`
}

type ItemDataPerson struct {
	CreatorType string `json:"creatorType"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
}

type ItemGitlab struct {
	LibraryId int64       `json:libraryid`
	Key       string      `json:"id"`
	Data      ItemGeneric `json:"data"`
	Meta      ItemMeta    `json:"meta"`
}

func (res *ItemCollectionCreateResult) checkSuccess(id int64) (string, error) {
	successlist := res.getSuccess()
	success, ok := successlist[id]
	if ok {
		return success, nil
	}
	unchangedlist := res.getUnchanged()
	unchanged, ok := unchangedlist[id]
	if ok {
		return unchanged, nil
	}
	failed := res.getFailed()
	fail, ok := failed[id]
	if ok {
		return fail.Key, errors.New(fmt.Sprintf("item %v update/creation failed with [%v]%v", fail.Key, fail.Code, fail.Message))
	}
	return "", errors.New(fmt.Sprintf("invalid id %v", id))
}

func (res *ItemCollectionCreateResult) getSuccess() map[int64]string {
	result := map[int64]string{}
	for key, val := range res.Success {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			continue
		}
		result[id] = val
	}
	return result
}

func (res *ItemCollectionCreateResult) getUnchanged() map[int64]string {
	result := map[int64]string{}
	for key, val := range res.Unchanged {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			continue
		}
		result[id] = val
	}
	return result
}

func (res *ItemCollectionCreateResult) getFailed() map[int64]ItemCollectionCreateResultFailed {
	result := map[int64]ItemCollectionCreateResultFailed{}
	for key, val := range res.Failed {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			continue
		}
		result[id] = val
	}
	return result
}

func (item *Item) GetType() (string, error) {
	return item.Data.ItemType, nil
}

func (item *Item) Backup(backupFs filesystem.FileSystem) error {
	var fname string
	var folder string
	if string(item.Data.ParentItem) != "" {
		folder = filepath.Clean(fmt.Sprintf("%v/items/%v", item.Group.Id, item.Data.ParentItem))
	} else {
		folder = filepath.Clean(fmt.Sprintf("%v/items", item.Group.Id))

	}
	fname = filepath.Clean(fmt.Sprintf("%v.json", item.Key))

	// write data to file
	data := struct {
		LibraryId int64       `json:"libraryid"`
		Id        string      `json:"id"`
		Data      interface{} `json:"data"`
		Meta      interface{} `json:"meta"`
	}{
		LibraryId: item.Group.Id,
		Id:        item.Key,
		Data:      item.Data,
		Meta:      item.Meta,
	}
	b, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return emperror.Wrapf(err, "cannot marshal data %v", data)
	}

	item.Group.Zot.Logger.Infof("storing #%v.%v to %v", item.Group.Id, item.Key, fname)
	if err := backupFs.FilePut(folder, fname, b, filesystem.FilePutOptions{}); err != nil {
		return emperror.Wrap(err, "cannot store data in file")
	}

	itemType, err := item.GetType()
	if err != nil {
		return emperror.Wrapf(err, "cannot get type of item %v/%v", item.Group.Id, item.Key)
	}
	if itemType == "attachment" && item.Data.MD5 != "" && !item.Deleted {
		bucket, err := item.Group.GetFolder()
		if err != nil {
			return emperror.Wrapf(err, "cannot get attachment folder")
		}

		item.Group.Zot.Logger.Infof("copying %v/%v/%v --> %v/%v/%v",
			item.Group.Zot.Fs.String(),
			bucket,
			item.Key,
			backupFs.String(),
			folder,
			fmt.Sprintf("%v.bin", item.Key),
		)
		file, err := item.Group.Zot.Fs.FileOpenRead(bucket, item.Key, filesystem.FileGetOptions{})
		if err != nil {
			return emperror.Wrapf(err, "cannot open file from %v/%v", bucket, item.Key)
		}
		defer file.Close()
		if err := backupFs.FileWrite(
			folder,
			fmt.Sprintf("%v.bin", item.Key),
			file,
			-1,
			filesystem.FilePutOptions{}); err != nil {
			return emperror.Wrapf(err, "cannot write to %v/%v.bin", folder, item.Key)
		}
	}

	return nil
}

func (item *Item) DownloadAttachmentCloud() (string, error) {
	bucket, err := item.Group.GetFolder()
	if err != nil {
		return "", emperror.Wrapf(err, "cannot get attachment folder")
	}
	//	filename := fmt.Sprintf("%s/%s", folder, item.Key)
	endpoint := fmt.Sprintf("/groups/%v/items/%s/file", item.Group.Id, item.Key)

	item.Group.Zot.Logger.Infof("rest call: %s", endpoint)
	call := item.Group.Zot.client.R().
		SetHeader("Accept", "application/json")
	var resp *resty.Response
	for {
		resp, err = call.Get(endpoint)
		if err != nil {
			return "", emperror.Wrapf(err, "cannot get current key from %s", endpoint)
		}
		if !item.Group.Zot.CheckRetry(resp.Header()) {
			break
		}
	}

	body := resp.Body()
	contentType := resp.Header().Get("Content-type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	//	_, err = item.Group.Zot.s3.PutObject(context.Background(), bucket, item.Key, bytes.NewReader(body), int64(len(body)), minio.PutObjectOptions{ContentType:contentType})
	if err := item.Group.Zot.Fs.FilePut(bucket, item.Key, body, filesystem.FilePutOptions{ContentType: contentType}); err != nil {
		return "", emperror.Wrap(err, "cannot put file")
	}

	md5str := resp.Header().Get("ETag")
	md5str = strings.Trim(md5str, "\"")
	if md5str == "" {
		md5sink := md5.New()
		md5str = fmt.Sprintf("%x", md5sink.Sum(resp.Body()))
	}
	item.Group.Zot.CheckBackoff(resp.Header())
	// we don't check. lets do it later
	/*
		if md5str != item.Data.MD5 {
			return "", errors.New(fmt.Sprintf("invalid checksum: %v != %v", md5str, item.Data.MD5))
		}
	*/
	/** gitlab sync not here
	if err := item.UploadAttachmentGitlab(body); err != nil {
		return "", emperror.Wrapf(err, "cannot upload attachment binary")
	}
	*/

	return md5str, nil
}

func (item *Item) uploadFileCloud() error {

	bucket, err := item.Group.GetFolder()
	if err != nil {
		return emperror.Wrapf(err, "cannot get attachment folder")
	}

	data, err := item.Group.Zot.Fs.FileGet(bucket, item.Key, filesystem.FileGetOptions{})
	if err != nil {
		return emperror.Wrapf(err, "cannot get content of %v/%v", bucket, item.Key)
	}

	md5str := fmt.Sprintf("%x", md5.Sum(data))
	if md5str == "" {
		return errors.New(fmt.Sprintf("cannot create md5 of %v/%v", bucket, item.Key))
	}
	if md5str == item.MD5 {
		// no change, do nothing
		return nil
	}

	/**
	Get Authorization
	*/
	endpoint := fmt.Sprintf("/groups/%v/%v/items/file", item.Group.Id, item.Key)
	h := item.Group.Zot.client.R().
		SetHeader("Content-Type", "application/x-www-form-urlencoded")
	if item.MD5 == "" {
		h.SetHeader("If-None-Match", "*")
	} else {
		h.SetHeader("If-Match", fmt.Sprintf("%s", item.MD5))
	}
	item.Group.Zot.Logger.Infof("rest call: POST %s", endpoint)
	info, err := item.Group.Zot.Fs.FileStat(bucket, item.Key, filesystem.FileStatOptions{})
	if err != nil {
		return emperror.Wrapf(err, "cannot stat file")
	}
	resp, err := h.
		SetFormData(map[string]string{
			"md5":      fmt.Sprintf("%s", md5str),
			"filename": item.Key,
			"filesize": fmt.Sprintf("%v", info.Size()),
			"mtime":    fmt.Sprintf("%v", info.ModTime().UnixNano()/int64(time.Millisecond)),
		}).
		Post(endpoint)
	if err != nil {
		return emperror.Wrapf(err, "upload attachment for item %v with %s", item.Key, endpoint)
	}
	switch resp.StatusCode() {
	case 200:
	case 403:
		return errors.New(fmt.Sprintf("file editing denied for item %v with %s", item.Key, endpoint))
	case 412:
		return errors.New(fmt.Sprintf("file precondition failed. please solve conflict for item %v with %s", item.Key, endpoint))
	case 413:
		return errors.New(fmt.Sprintf("file too large. please upgrade storage for item %v with %s", item.Key, endpoint))
	case 428:
		return errors.New(fmt.Sprintf("file precondition required. If-Match or If-None-Match was not provided for item %v with %s", item.Key, endpoint))
	case 429:
		return errors.New(fmt.Sprintf("file too many requests. Too many unfinished uploads for item %v with %s", item.Key, endpoint))
	default:
		return errors.New(fmt.Sprintf("file unknown error #%v for item %v with %s", resp.Status(), item.Key, endpoint))
	}
	jsonstr := resp.Body()
	var result map[string]string
	if err = json.Unmarshal(jsonstr, &result); err != nil {
		return emperror.Wrapf(err, "cannot unmarshall result %s", string(jsonstr))
	}
	var ok bool
	if _, ok = result["exists"]; ok {
		// already there
		return nil
	}

	/**
	Upload file (Amazon S3 e.a.)
	*/
	endpoint, ok = result["url"]
	if !ok {
		return errors.New(fmt.Sprintf("no url in upload authorization %v", string(jsonstr)))
	}
	contenttype, ok := result["contentType"]
	if !ok {
		return errors.New(fmt.Sprintf("no contentType in upload authorization %v", string(jsonstr)))
	}
	prefix, ok := result["prefix"]
	if !ok {
		return errors.New(fmt.Sprintf("no prefix in upload authorization %v", string(jsonstr)))
	}
	suffix, ok := result["suffix"]
	if !ok {
		return errors.New(fmt.Sprintf("no suffix in upload authorization %v", string(jsonstr)))
	}
	uploadKey, ok := result["uploadKey"]
	if !ok {
		return errors.New(fmt.Sprintf("no uploadKey in upload authorization %v", string(jsonstr)))
	}
	item.Group.Zot.Logger.Infof("rest call: POST %s", endpoint)
	resp, err = resty.New().R().
		SetHeader("Content-Type", contenttype).
		SetBody(append([]byte(prefix), append(data, []byte(suffix)...)...)).
		Post(endpoint)
	if err != nil {
		return emperror.Wrapf(err, "error uploading file to %v", endpoint)
	}
	if resp.StatusCode() != 201 {
		return errors.New(fmt.Sprintf("error uploading file with status %v - %v", resp.Status(), resp.Body()))
	}

	/**
	register upload
	*/
	endpoint = fmt.Sprintf("/groups/%v/%v/items/file", item.Group.Id, item.Key)
	item.Group.Zot.Logger.Infof("rest call: POST %s", endpoint)
	h = item.Group.Zot.client.R()
	if item.MD5 == "" {
		h.SetHeader("If-None-Match", "*")
	} else {
		h.SetHeader("If-Match", fmt.Sprintf("%s", item.MD5))
	}
	resp, err = h.
		SetFormData(map[string]string{"upload": uploadKey}).
		Post(endpoint)
	if err != nil {
		return emperror.Wrapf(err, "cannot register upload %v", endpoint)
	}
	switch resp.StatusCode() {
	case 204:
	case 412:
		return errors.New(fmt.Sprintf("Precondition failed - The file has changed remotely since retrieval for item %v.%v", item.Group.Id, item.Key))
	}
	// todo: should be etag from upload...
	item.MD5 = md5str
	return nil
}

func (item *Item) UpdateCloud(lastModifiedVersion *int64) error {
	item.Group.Zot.Logger.Infof("Creating Zotero Item [#%s]", item.Key)
	/*
		if item.Version > 0 {
			item.Version = *lastModifiedVersion
		}

	*/
	item.Data.Version = item.Version
	if item.Deleted {
		endpoint := fmt.Sprintf("/groups/%v/%v/items", item.Group.Id, item.Key)
		item.Group.Zot.Logger.Infof("rest call: DELETE %s", endpoint)
		resp, err := item.Group.Zot.client.R().
			SetHeader("Accept", "application/json").
			SetHeader("If-Unmodified-Since-Version", fmt.Sprintf("%v", *lastModifiedVersion)).
			Delete(endpoint)
		if err != nil {
			return emperror.Wrapf(err, "create item %v with %s", item.Key, endpoint)
		}
		switch resp.RawResponse.StatusCode {
		case 409:
			return errors.New(fmt.Sprintf("delete: Conflict: the target library #%v is locked", item.Group.Id))
		case 412:
			return errors.New(fmt.Sprintf("delete: Precondition failed: The item #%v.%v has changed since retrieval", item.Group.Id, item.Key))
		case 428:
			return errors.New(fmt.Sprintf("delete: Precondition required: If-Unmodified-Since-Version was not provided."))
		}
	} else {
		endpoint := fmt.Sprintf("/groups/%v/items", item.Group.Id)
		item.Group.Zot.Logger.Infof("rest call: POST %s", endpoint)
		items := []ItemGeneric{item.Data}
		req := item.Group.Zot.client.R().
			SetHeader("Accept", "application/json").
			SetHeader("If-Unmodified-Since-Version", fmt.Sprintf("%v", *lastModifiedVersion)).
			SetBody(items)

		resp, err := req.
			Post(endpoint)
		if err != nil {
			return emperror.Wrapf(err, "create item %v with %s", item.Key, endpoint)
		}
		result := ItemCollectionCreateResult{}
		jsonstr := resp.Body()
		h, err := strconv.ParseInt(resp.RawResponse.Header.Get("Last-Modified-Version"), 10, 64)
		if err != nil {
			*lastModifiedVersion++
		}
		if h > *lastModifiedVersion {
			*lastModifiedVersion = h
		}
		if err := json.Unmarshal(jsonstr, &result); err != nil {
			return emperror.Wrapf(err, "cannot unmarshall result %s", string(jsonstr))
		}
		//		*lastModifiedVersion += int64(len(result.Unchanged)) + int64(len(result.Success)) + int64(len(result.Failed))

		successKey, err := result.checkSuccess(0)
		if err != nil {
			item.Version = *lastModifiedVersion
			item.Data.Version = *lastModifiedVersion
			item.Status = SyncStatus_Modified
			item.UpdateLocal()
			return emperror.Wrapf(err, "could not create item #%v.%v", item.Group.Id, item.Key)
		}

		for _, i := range result.Successful {
			i.Group = item.Group
			i.UpdateLocal()
			//item.Group.Zot.Logger.Infof( "%v: %v", key, i)
			if item.Group.ItemVersion < i.Version {
				item.Group.ItemVersion = i.Version
			}
		}
		if successKey != item.Key {
			return errors.New(fmt.Sprintf("invalid key %s. source key: %s", successKey, item.Key))
		}
		if item.Data.ItemType == "attachment" && item.Data.LinkMode == "imported_file" {
			if err := item.uploadFileCloud(); err != nil {
				return emperror.Wrapf(err, "cannot upload file")
			}
		}
	}
	item.Status = SyncStatus_Synced
	if err := item.UpdateLocal(); err != nil {
		return errors.New(fmt.Sprintf("cannot store item in db %v.%v", item.Group.Id, item.Key))
	}
	return nil
}

func (item *Item) UpdateLocal() error {
	item.Group.Zot.Logger.Infof("Updating Item [#%s]", item.Key)

	md5val := sql.NullString{Valid: false}
	itemType, err := item.GetType()
	if err != nil {
		return emperror.Wrapf(err, "cannot get item type")
	}
	// if not deleted and status is synced get the attachment
	if itemType == "attachment" && item.Data.MD5 != "" && !item.Deleted && item.Status == SyncStatus_Synced {
		md5val.String, err = item.DownloadAttachmentCloud()
		if err != nil {
			return emperror.Wrapf(err, "cannot download attachment")
		}
	} else {
		md5val.String = item.MD5
		md5val.Valid = true
	}
	md5val.Valid = md5val.String != ""

	data, err := json.Marshal(item.Data)
	if err != nil {
		return emperror.Wrapf(err, "cannot marshall data %v", item.Data)
	}
	meta, err := json.Marshal(item.Meta)
	if err != nil {
		return emperror.Wrapf(err, "cannot marshall meta %v", item.Meta)
	}
	var sqlstr string
	var params []interface{}
	if item.Version > 0 {
		sqlstr = fmt.Sprintf("UPDATE %s.items SET version=$1, data=$2, meta=$3, trashed=$4, deleted=$5, sync=$6, md5=$7, modified=NOW() "+
			"WHERE library=$8 AND key=$9", item.Group.Zot.dbSchema)
		params = []interface{}{
			item.Version,
			string(data),
			string(meta),
			item.Trashed,
			item.Deleted,
			SyncStatusString[SyncStatus_Modified],
			md5val,
			item.Group.Id,
			item.Key,
		}
	} else {
		sqlstr = fmt.Sprintf("UPDATE %s.items SET data=$1, meta=$2, trashed=$3, deleted=$4, sync=$5, md5=$6, modified=NOW() "+
			"WHERE library=$7 AND key=$8", item.Group.Zot.dbSchema)
		params = []interface{}{
			string(data),
			string(meta),
			item.Trashed,
			item.Deleted,
			SyncStatusString[item.Status],
			md5val,
			item.Group.Id,
			item.Key,
		}
	}
	_, err = item.Group.Zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	return nil
}

func (item *Item) GetChildrenLocal() (*[]Item, error) {
	item.Group.Zot.Logger.Infof("get children of  item [#%s]", item.Key)
	sqlstr := fmt.Sprintf("SELECT i.key, i.version, i.data, i.meta, i.trashed, i.deleted, i.sync, i.md5, i.gitlab"+
		" FROM %s.items i, %s.item_type_hier ith"+
		" WHERE i.key=ith.key AND i.library=ith.library AND i.library=$1 AND ith.parent=$2", item.Group.Zot.dbSchema, item.Group.Zot.dbSchema)
	params := []interface{}{
		item.Group.Id,
		item.Key,
	}
	rows, err := item.Group.Zot.db.Query(sqlstr, params...)
	if err != nil {
		if err == sql.ErrNoRows {
			return &[]Item{}, nil
		}
		return &[]Item{}, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}
	defer rows.Close()
	items := []Item{}
	for rows.Next() {
		i, err := item.Group.itemFromRow(rows)
		if err != nil {
			return &[]Item{}, emperror.Wrapf(err, "cannot scan result row")
		}
		items = append(items, *i)
	}

	return &items, nil
}

func (item *Item) DeleteLocal() error {
	item.Group.Zot.Logger.Infof("deleting item [#%s]", item.Key)
	children, err := item.GetChildrenLocal()
	if err != nil {
		return emperror.Wrapf(err, "cannot get children of #%v", item.Key)
	}
	for _, c := range *children {
		if err := c.DeleteLocal(); err != nil {
			return emperror.Wrapf(err, "cannot delete child #%v of #%v", c.Key, item.Key)
		}
	}
	item.Deleted = true
	item.Status = SyncStatus_Modified
	if err := item.UpdateLocal(); err != nil {
		return emperror.Wrapf(err, "cannot store item #%v", item.Key)
	}
	return nil
}
