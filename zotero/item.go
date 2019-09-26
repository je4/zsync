package zotero

import (
	"crypto/md5"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/goph/emperror"
	"gopkg.in/resty.v1"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"
)

type ItemMeta struct {
	CreatedByUser  User   `json:"createdByUser"`
	CreatorSummary string `json:"creatorSummary,omitempty"`
	ParsedDate     string `json:"parsedDate,omitempty"`
	NumChildren    int64  `json:"numChildren,omitempty"`
}

type Item struct {
	Key     string            `json:"key"`
	Version int64             `json:"version"`
	Library CollectionLibrary `json:"library,omitempty"`
	Links   interface{}       `json:"links,omitempty"`
	Meta    ItemMeta          `json:"meta,omitempty"`
	Data    ItemGeneric       `json:"data,omitempty"`
	Group   *Group            `json:"-"`
	Trashed bool              `json:"-"`
	Deleted bool              `json:"-"`
	Status  SyncStatus        `json:"-"`
	MD5     string            `json:"-"`
}

type ItemTag struct {
	Tag  string `json:"tag"`
	Type int64  `json:"type,omitempty"`
}

type ItemDataBase struct {
	Key          string            `json:"key,omitempty"`
	Version      int64             `json:"version"`
	ItemType     string            `json:"itemType"`
	Tags         []ItemTag         `json:"tags"`
	Relations    map[string]string `json:"relations"`
	ParentItem   string            `json:"parentItem,omitempty"`
	Collections  []string          `json:"collections"`
	DateAdded    string            `json:"dateAdded,omitempty"`
	DateModified string            `json:"dateModified,omitempty"`
}

type ItemDataPerson struct {
	CreatorType string `json:"creatorType"`
	FirstName   string `json:"firstName"`
	LastName    string `json:"lastName"`
}

type ItemCreateResultFailed struct {
	Key     string `json:"key"`
	Code    int64  `json:"code"`
	Message string `json:"message"`
}

type ItemCreateResult struct {
	Success   map[string]string                 `json:"success"`
	Unchanged map[string]string                 `json:"unchanged"`
	Failed    map[string]ItemCreateResultFailed `json:"failed"`
}

func (res *ItemCreateResult) checkSuccess(id int64) (string, error) {
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

func (res *ItemCreateResult) getSuccess() map[int64]string {
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

func (res *ItemCreateResult) getUnchanged() map[int64]string {
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

func (res *ItemCreateResult) getFailed() map[int64]ItemCreateResultFailed {
	result := map[int64]ItemCreateResultFailed{}
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

func (item *Item) StoreAttachment() (string, error) {
	folder, err := item.Group.GetAttachmentFolder()
	if err != nil {
		return "", emperror.Wrapf(err, "cannot get attachment folder")
	}
	filename := fmt.Sprintf("%s/%s", folder, item.Key)
	endpoint := fmt.Sprintf("/groups/%v/items/%s/file", item.Group.Id, item.Key)

	item.Group.zot.logger.Infof("rest call: %s", endpoint)
	resp, err := item.Group.zot.client.R().
		SetHeader("Accept", "application/json").
		Get(endpoint)
	if err != nil {
		return "", emperror.Wrapf(err, "cannot get current key from %s", endpoint)
	}
	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		return "", emperror.Wrapf(err, "cannot create %v", filename)
	}
	defer f.Close()
	f.Write(resp.Body())
	md5str := resp.Header().Get("ETag")
	md5str = strings.Trim(md5str, "\"")
	if md5str == "" {
		md5sink := md5.New()
		md5str = fmt.Sprintf("%x", md5sink.Sum(resp.Body()))
	}
	item.Group.zot.CheckWait(resp.Header())
	return md5str, nil
}
func (item *Item) Update() error {
	item.Group.zot.logger.Infof("Creating Zotero Item [#%s]", item.Key)

	item.Data.Version = item.Version
	if item.Deleted {
		endpoint := fmt.Sprintf("/groups/%v/items/%v", item.Group.Id, item.Key)
		item.Group.zot.logger.Infof("rest call: DELETE %s", endpoint)
		resp, err := item.Group.zot.client.R().
			SetHeader("Accept", "application/json").
			SetHeader("If-Unmodified-Since-Version", fmt.Sprintf("%v", item.Version)).
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
		item.Group.zot.logger.Infof("rest call: POST %s", endpoint)
		items := []ItemGeneric{item.Data}
		resp, err := item.Group.zot.client.R().
			SetHeader("Accept", "application/json").
			SetBody(items).
			Post(endpoint)
		if err != nil {
			return emperror.Wrapf(err, "create item %v with %s", item.Key, endpoint)
		}
		result := ItemCreateResult{}
		jsonstr := resp.Body()
		if err := json.Unmarshal(jsonstr, &result); err != nil {
			return emperror.Wrapf(err, "cannot unmarshall result %s", string(jsonstr))
		}
		successKey, err := result.checkSuccess(0)
		if err != nil {
			return emperror.Wrapf(err, "could not create item #%v.%v", item.Group.Id, item.Key)
		}
		if successKey != item.Key {
			return errors.New(fmt.Sprintf("invalid key %s. source key: %s", successKey, item.Key))
		}
		if item.Data.ItemType == "attachment" {
			// todo: attachment upload
			var md5str string
			var finfo os.FileInfo
			//var md5Sink hash.Hash
			var endpoint string
			var resp *resty.Response
			var h *resty.Request
			// var f *os.File
			var jsonstr []byte
			var result map[string]string
			var attachmentFolder string
			var attachmentFile string
			var attachmentBytes []byte
			var ok bool
			var contenttype string
			var prefix string
			var suffix string
			var uploadKey string

			attachmentFolder, err = item.Group.GetAttachmentFolder()
			if err != nil {
				goto End
			}
			attachmentFile = fmt.Sprintf("%s/%s", attachmentFolder, item.Key)

			finfo, err = os.Stat(attachmentFile)
			if err != nil {
				goto End
			}

			attachmentBytes, err = ioutil.ReadFile(attachmentFile)
			if err != nil {
				goto End
			}
			md5str = fmt.Sprintf("%x", md5.Sum(attachmentBytes))
			if md5str == "" || md5str == item.MD5 {
				goto End
			}

			/**
			Get Authorization
			 */
			endpoint = fmt.Sprintf("/groups/%v/items/%v/file", item.Group.Id, item.Key)
			h = item.Group.zot.client.R().
				SetHeader("Content-Type", "application/x-www-form-urlencoded")
			if item.MD5 == "" {
				h.SetHeader("If-None-Match", "*")
			} else {
				h.SetHeader("If-Match", fmt.Sprintf("%s", item.MD5))
			}
			item.Group.zot.logger.Infof("rest call: POST %s", endpoint)
			resp, err = h.
				SetFormData(map[string]string{
					"md5":      fmt.Sprintf("%s", md5str),
					"filename": item.Key,
					"filesize": fmt.Sprintf("%v", finfo.Size()),
					"mtime":    fmt.Sprintf("%v", finfo.ModTime().UnixNano()/int64(time.Millisecond)),
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
			jsonstr = resp.Body()
			if err = json.Unmarshal(jsonstr, &result); err != nil {
				return emperror.Wrapf(err, "cannot unmarshall result %s", string(jsonstr))
			}
			if _, ok = result["exists"]; ok {
				// already there
				goto End
			}

			/**
			Upload file
			 */
			endpoint, ok = result["url"]
			if !ok {
				return errors.New(fmt.Sprintf("no url in upload authorization %v", string(jsonstr)))
			}
			contenttype, ok = result["contentType"]
			if !ok {
				return errors.New(fmt.Sprintf("no contentType in upload authorization %v", string(jsonstr)))
			}
			prefix, ok = result["prefix"]
			if !ok {
				return errors.New(fmt.Sprintf("no prefix in upload authorization %v", string(jsonstr)))
			}
			suffix, ok = result["suffix"]
			if !ok {
				return errors.New(fmt.Sprintf("no suffix in upload authorization %v", string(jsonstr)))
			}
			uploadKey, ok = result["uploadKey"]
			if !ok {
				return errors.New(fmt.Sprintf("no uploadKey in upload authorization %v", string(jsonstr)))
			}
			item.Group.zot.logger.Infof("rest call: POST %s", endpoint)
			resp, err = resty.New().R().
				SetHeader("Content-Type", contenttype).
				SetBody(append([]byte(prefix), append(attachmentBytes, []byte(suffix)...)...)).
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
			endpoint = fmt.Sprintf("/groups/%v/items/%v/file", item.Group.Id, item.Key)
			item.Group.zot.logger.Infof("rest call: POST %s", endpoint)
			h = item.Group.zot.client.R()
			if item.MD5 == "" {
				h.SetHeader("If-None-Match", "*")
			} else {
				h.SetHeader("If-Match", fmt.Sprintf("%s", item.MD5))
			}
			resp, err = h.
				SetFormData(map[string]string{"upload":uploadKey}).
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
		End:
		}
	}
	item.Status = SyncStatus_Synced
	if err := item.UpdateDB(); err != nil {
		return errors.New(fmt.Sprintf("cannot store item in db %v.%v", item.Group.Id, item.Key))
	}
	return nil
}

func (item *Item) UpdateDB() error {
	item.Group.zot.logger.Infof("Updating Item [#%s]", item.Key)

	md5val := sql.NullString{Valid: false}
	itemType, err := item.GetType()
	if err != nil {
		return emperror.Wrapf(err, "cannot get item type")
	}
	// if not deleted and status is synced get the attachment
	if itemType == "attachment" && !item.Deleted && item.Status == SyncStatus_Synced {
		md5val.String, err = item.StoreAttachment()
		if err != nil {
			return emperror.Wrapf(err, "cannot download attachment")
		}
		if md5val.String != "" {
			md5val.Valid = true
		}
	} else {
		md5val.String = item.MD5
		md5val.Valid = true
	}

	data, err := json.Marshal(item.Data)
	if err != nil {
		return emperror.Wrapf(err, "cannot marshall data %v", item.Data)
	}
	meta, err := json.Marshal(item.Meta)
	if err != nil {
		return emperror.Wrapf(err, "cannot marshall meta %v", item.Meta)
	}
	sqlstr := fmt.Sprintf("UPDATE %s.items SET version=$1, data=$2, meta=$3, trashed=$4, deleted=$5, sync=$6, md5=$7 "+
		"WHERE library=$8 AND key=$9", item.Group.zot.dbSchema)
	params := []interface{}{
		item.Version,
		string(data),
		string(meta),
		item.Trashed,
		item.Deleted,
		SyncStatusString[item.Status],
		md5val.String,
		item.Group.Id,
		item.Key,
	}
	_, err = item.Group.zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
	}

	return nil
}
