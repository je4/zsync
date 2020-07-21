package zotero

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/goph/emperror"
	"github.com/lib/pq"
	"github.com/op/go-logging"
	"github.com/xanzy/go-gitlab"
	"gitlab.fhnw.ch/hgk-dima/zotero-sync/pkg/filesystem"
	"gopkg.in/resty.v1"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type Zotero struct {
	baseUrl          *url.URL
	apiKey           string
	client           *resty.Client
	logger           *logging.Logger
	db               *sql.DB
	dbSchema         string
	attachmentFolder string
	newGroupActive   bool
	CurrentKey       *ApiKey
	git              *gitlab.Client
	gitProject       *gitlab.Project
	fs               filesystem.FileSystem
}


type Library struct {
	Type  string      `json:"type"`
	Id    int64       `json:"id"`
	Name  string      `json:"name"`
	Links interface{} `json:"links"`
}

type ItemCollectionCreateResultFailed struct {
	Key     string `json:"key"`
	Code    int64  `json:"code"`
	Message string `json:"message"`
}

type ItemCollectionCreateResult struct {
	Success    map[string]string                           `json:"success"`
	Unchanged  map[string]string                           `json:"unchanged"`
	Failed     map[string]ItemCollectionCreateResultFailed `json:"failed"`
	Successful map[string]Item                             `json:"successful"`
}

type Deletions struct {
	Collections []string `json:"collections"`
	Searches    []string `json:"searches"`
	Items       []string `json:"items"`
	Tags        []string `json:"tags"`
	Settings    []string `json:"settings"`
}

// Relations are empty array or string map
type RelationList map[string]string

func (rl *RelationList) UnmarshalJSON(data []byte) error {
	var i interface{}
	if err := json.Unmarshal(data, &i); err != nil {
		return err
	}
	switch d := i.(type) {
	case map[string]interface{}:
		*rl = RelationList{}
		for key, val := range d {
			(*rl)[key], _ = val.(string)
		}
	case []interface{}:
		if len(d) > 0 {
			return errors.New(fmt.Sprintf("invalid object list for type RelationList - %s", string(data)))
		}
	}
	return nil
}

// zotero returns single item lists as string
type ZoteroStringList []string

func (irl *ZoteroStringList) UnmarshalJSON(data []byte) error {
	var i interface{}
	if err := json.Unmarshal(data, &i); err != nil {
		return err
	}
	switch i.(type) {
	case string:
		*irl = ZoteroStringList{i.(string)}
	case []interface{}:
		*irl = ZoteroStringList{}
		for _, i2 := range i.([]interface{}) {
			str, ok := i2.(string)
			if !ok {
				errors.New(fmt.Sprintf("invalid type %v for %v", reflect.TypeOf(i2), i2))
			}
			*irl = append(*irl, str)
		}
	default:
		return errors.New(fmt.Sprintf("invalid type %v for %v", reflect.TypeOf(i), string(data)))
	}
	return nil
}

// zotero treats empty strings as false in ParentCollection
type Parent string

func (pc *Parent) UnmarshalJSON(data []byte) error {
	var i interface{}
	if err := json.Unmarshal(data, &i); err != nil {
		return err
	}
	switch i.(type) {
	case bool:
		*pc = ""
	case string:
		*pc = Parent(i.(string))
	default:
		return errors.New(fmt.Sprintf("invalid no string for %v", string(data)))
	}
	return nil
}

func IsEmptyResult(err error) bool {
	return err == sql.ErrNoRows
}

func IsUniqueViolation(err error, constraint string) bool {
	pqErr, ok := err.(*pq.Error)
	if !ok {
		return false
	}
	if constraint != "" && pqErr.Constraint != constraint {
		return false
	}
	return pqErr.Code == "23505"
}

func NewZotero(baseUrl string, apiKey string, db *sql.DB, fs filesystem.FileSystem, dbSchema string, attachmentFolder string, newGroupActive bool, git *gitlab.Client, gitProject *gitlab.Project, logger *logging.Logger, dbOnly bool) (*Zotero, error) {
	burl, err := url.Parse(baseUrl)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot create url from %s", baseUrl)
	}
	zot := &Zotero{
		baseUrl:          burl,
		apiKey:           apiKey,
		logger:           logger,
		db:               db,
		fs:               fs,
		dbSchema:         dbSchema,
		attachmentFolder: attachmentFolder,
		newGroupActive:   newGroupActive,
		git:              git,
		gitProject:       gitProject,
	}
	if !dbOnly {
		err = zot.Init()
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot init zotero")
		}
	}
	return zot, err
}

func (zot *Zotero) Init() (err error) {
	zot.client = resty.New()
	zot.client.SetHostURL(zot.baseUrl.String())
	zot.client.SetAuthToken(zot.apiKey)
	zot.client.SetContentLength(true)
	zot.client.SetRedirectPolicy(resty.FlexibleRedirectPolicy(3))
	zot.CurrentKey, err = zot.getCurrentKey()

	return
}

/**
Clients accessing the Zotero API should be prepared to handle two forms of rate limiting: backoff requests and hard limiting.
If the API servers are overloaded, the API may include a Backoff: <seconds> HTTP header in responses, indicating that the client should perform the minimum number of requests necessary to maintain data consistency and then refrain from making further requests for the number of seconds indicated. Backoff can be included in any response, including successful ones.
If a client has made too many requests within a given time period, the API may return 429 Too Many Requests with a Retry-After: <seconds> header. Clients receiving a 429 should wait the number of seconds indicated in the header before retrying the request.
Retry-After can also be included with 503 Service Unavailable responses when the server is undergoing maintenance.
*/
func (zot *Zotero) CheckRetry(header http.Header) bool {
	var err error
	retryAfter := int64(0)
	retryAfterStr := header.Get("Retry-After")
	if retryAfterStr != "" {
		retryAfter, err = strconv.ParseInt(retryAfterStr, 10, 64)
		if err != nil {
			retryAfter = 0
		}
	}

	if retryAfter > 0 {
		zot.logger.Infof("Sleeping %v seconds (RetryAfter)", retryAfter)
		time.Sleep(time.Duration(retryAfter) * time.Second)
	}
	return retryAfter > 0
}

func (zot *Zotero) CheckBackoff(header http.Header) bool {
	var err error
	backoff := int64(0)
	backoffStr := header.Get("Backoff")
	if backoffStr != "" {
		backoff, err = strconv.ParseInt(backoffStr, 10, 64)
		if err != nil {
			backoff = 0
		}
	}
	if backoff > 0 {
		zot.logger.Infof("Sleeping %v seconds (Backoff)", backoff)
		time.Sleep(time.Duration(backoff) * time.Second)
	}
	return backoff > 0
}

func (zot *Zotero) GetTypeStructs() (str string) {
	type itemType struct {
		ItemType  string `json:"itemType"`
		Localizes string `json:"localized"`
	}

	type field struct {
		Field     string `json:"field"`
		Localized string `json:"localized"`
	}

	endpoint := "/itemTypes"
	call := zot.client.R().
		SetHeader("Accept", "application/json")
	var resp *resty.Response
	var err error
	for {
		resp, err = call.Get(endpoint)
		if err != nil {
			zot.logger.Panicf("cannot execute rest call to %s", endpoint)
		}
		if zot.CheckRetry(resp.Header()) {
			break
		}
	}
	rawBody := resp.Body()
	types := []itemType{}
	if err := json.Unmarshal(rawBody, &types); err != nil {
		zot.logger.Panicf("cannot unmarshal %s: %v", string(rawBody), err)
	}
	zot.CheckBackoff(resp.Header())
	str += fmt.Sprintln("switch item.(type) {")
	for _, _type := range types {
		str += fmt.Sprintf("case Item%s:\n", strings.Title(_type.ItemType))
	}

	for _, _type := range types {
		str += fmt.Sprintf("type Item%s struct {\n", strings.Title(_type.ItemType))
		str += fmt.Sprintln("	ItemDataBase")
		str += fmt.Sprintln("	Creators        []ItemDataPerson `json:\"creators\"`")

		endpoint = "/itemTypeFields"
		call := zot.client.R().
			SetHeader("Accept", "application/json").
			SetQueryParam("itemType", _type.ItemType)
		var resp *resty.Response
		var err error
		for {
			resp, err = call.Get(endpoint)
			if err != nil {
				zot.logger.Panicf("cannot execute rest call to %s", endpoint)
			}
			if !zot.CheckRetry(resp.Header()) {
				break
			}
		}
		rawBody := resp.Body()
		fields := []field{}
		if err := json.Unmarshal(rawBody, &fields); err != nil {
			zot.logger.Panicf("cannot unmarshal %s: %v", string(rawBody), err)
		}
		zot.CheckBackoff(resp.Header())
		for _, field := range fields {
			str += fmt.Sprintf("   %s string `json:\"%s,omitempty\"` // %s\n", strings.Title(field.Field), field.Field, field.Localized)
		}
		str += fmt.Sprintln("}")
		str += fmt.Sprintln("")
	}
	return
}

func (zot *Zotero) GetGroupCloud(groupId int64) (*Group, error) {
	endpoint := fmt.Sprintf("/groups/%v", groupId)
	zot.logger.Infof("rest call: %s", endpoint)

	call := zot.client.R().
		SetHeader("Accept", "application/json")
	var resp *resty.Response
	var err error
	for {
		resp, err = call.Get(endpoint)
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot get current key from %s", endpoint)
		}
		if !zot.CheckRetry(resp.Header()) {
			break
		}
	}
	rawBody := resp.Body()
	group := &Group{}
	if err := json.Unmarshal(rawBody, group); err != nil {
		return nil, emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	zot.CheckBackoff(resp.Header())
	group.zot = zot
	return group, nil
}

func (zot *Zotero) DeleteCollectionDB(key string) error {
	sqlstr := fmt.Sprintf("UPDATE %s.collections SET deleted=true WHERE key=$1", zot.dbSchema)

	params := []interface{}{
		key,
	}
	if _, err := zot.db.Exec(sqlstr, params...); err != nil {
		return emperror.Wrapf(err, "error executing %s: %v", sqlstr, params)
	}
	return nil
}

func (zot *Zotero) deleteGitlab(filename string, branch string, commit string) (gitlab.EventTypeValue, error) {
	if zot.git == nil {
		return gitlab.DestroyedEventType, nil
	}
	gopt := gitlab.DeleteFileOptions{
		Branch:        &branch,
		AuthorEmail:   nil,
		AuthorName:    nil,
		CommitMessage: &commit,
	}
	_, err := zot.git.RepositoryFiles.DeleteFile(zot.gitProject.ID, filename, &gopt)
	if err != nil {
		return gitlab.DestroyedEventType, emperror.Wrapf(err, "canot delete file %v", filename)
	}
	return gitlab.DestroyedEventType, nil
}

func (zot *Zotero) uploadGitlab(filename string, branch string, commit string, enc string, data string) (gitlab.EventTypeValue, error) {
	if zot.git == nil {
		return gitlab.CreatedEventType, nil
	}
	zot.logger.Infof("uploading %v to gitlab (%vbytes)", commit, len(data))

	var e *string
	if enc == "" {
		e = nil
	} else {
		e = &enc
	}
	gopt := gitlab.CreateFileOptions{
		Branch:        &branch,
		Encoding:      e,
		AuthorEmail:   nil,
		AuthorName:    nil,
		Content:       &data,
		CommitMessage: &commit,
	}
	fileinfo, _, err := zot.git.RepositoryFiles.CreateFile(zot.gitProject.ID, filename, &gopt)
	if err != nil {
		glErr, ok := err.(*gitlab.ErrorResponse)
		if !ok {
			return gitlab.ClosedEventType, emperror.Wrapf(err, "upload on gitlab failed")
		}
		if glErr.Response.StatusCode != http.StatusBadRequest {
			return gitlab.ClosedEventType, emperror.Wrapf(err, "upload on gitlab failed")
		}
		if !strings.Contains(glErr.Message, "file with this name already exists") {
			return gitlab.ClosedEventType, emperror.Wrapf(err, "upload on gitlab failed")
		}
		zot.logger.Debugf("%v already exists. updating...", filename)

		gopt := gitlab.UpdateFileOptions{
			Branch:        &branch,
			Encoding:      e,
			AuthorEmail:   nil,
			AuthorName:    nil,
			Content:       &data,
			CommitMessage: &commit,
		}
		fileinfo, _, err = zot.git.RepositoryFiles.UpdateFile(zot.gitProject.ID, filename, &gopt)
		if err != nil {
			return gitlab.ClosedEventType, emperror.Wrapf(err, "update on gitlab failed")
		}
		return gitlab.UpdatedEventType, nil
	}
	zot.logger.Debugf("uploading %v to gitlab done (%vbytes) - %v", commit, len(data), fileinfo.String())
	return gitlab.CreatedEventType, nil
}

func (zot *Zotero) groupFromRow(rowss interface{}) (*Group, error) {

	group := Group{}
	var datastr sql.NullString
	var gitlab sql.NullTime
	switch rowss.(type) {
	case *sql.Row:
		row := rowss.(*sql.Row)
		if err := row.Scan(&group.Id, &group.Version, &datastr, &group.Deleted, &gitlab, &group.CollectionVersion, &group.ItemVersion, &group.TagVersion); err != nil {
			if err == sql.ErrNoRows {
				return nil, nil
			}
			return nil, emperror.Wrapf(err, "cannot scan row")
		}
	case *sql.Rows:
		rows := rowss.(*sql.Rows)
		if err := rows.Scan(&group.Id, &group.Version, &datastr, &group.Deleted, &gitlab, &group.CollectionVersion, &group.ItemVersion, &group.TagVersion); err != nil {
			return nil, emperror.Wrapf(err, "cannot scan row")
		}
	default:
		return nil, errors.New(fmt.Sprintf("unknown row type: %v", reflect.TypeOf(rowss).String()))
	}
	if gitlab.Valid {
		group.Gitlab = &gitlab.Time
	}
	if datastr.Valid {
		if err := json.Unmarshal([]byte(datastr.String), &group.Data); err != nil {
			return nil, emperror.Wrapf(err, "cannot ummarshall data %s", datastr.String)
		}
	} else {
		return nil, errors.New(fmt.Sprintf("Group has no data %v", group.Id))
	}

	return &group, nil
}

func (zot *Zotero) gitlabCheck(path, ref string) (bool, error) {
	opts := &gitlab.GetFileMetaDataOptions{Ref: &ref}
	_, resp, err := zot.git.RepositoryFiles.GetFileMetaData(zot.gitProject.ID, path, opts )
	if err != nil {
		if errResp, ok := err.(*gitlab.ErrorResponse); ok {
			if errResp.Response.StatusCode != http.StatusNotFound {
				return false, emperror.Wrapf(err, "cannot check existence of %v", path)
			}
		} else {
			return false, emperror.Wrapf(err, "cannot check existence of %v", path)
		}
	}
	return resp.StatusCode != http.StatusNotFound, nil
}

func (zot *Zotero) SyncGroupsGitlab() error {
	synctime := time.Now()
	sqlstr := fmt.Sprintf("SELECT id, version, data, deleted, gitlab, itemversion, collectionversion, tagversion"+
		" FROM %s.groups"+
		" WHERE (gitlab < modified OR gitlab is null)", zot.dbSchema)
	rows, err := zot.db.Query(sqlstr)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s", sqlstr)
	}

	result := []Group{}
	for rows.Next() {
		group, err := zot.groupFromRow(rows)
		if err != nil {
			continue
		}
		group.zot = zot
		result = append(result, *group)
	}
	rows.Close()

	num := int64(len(result))
	slices := num / 20
	if num%20 > 0 {
		slices++
	}
	for i := int64(0); i < slices; i++ {
		start := i * 20
		end := i*20 + 20
		if num < end {
			end = num
		}
		parts := result[start:end]
		gbranch := "master"
		gaction := []*gitlab.CommitAction{}
		var creations int64
		var deletions int64
		var updates int64
		for _, group := range parts {
			// new and deleted -> we will not upload
			if group.Gitlab == nil && group.Deleted {
				continue
			}

			glGroup := GroupGitlab{
				Id:                group.Id,
				Data:              group.Data,
				CollectionVersion: group.CollectionVersion,
				ItemVersion:       group.ItemVersion,
				TagVersion:        group.TagVersion,
			}

			data, err := json.Marshal(glGroup)
			if err != nil {
				return emperror.Wrapf(err, "cannot marshall data %v", group.Data)
			}
			var prettyJSON bytes.Buffer
			err = json.Indent(&prettyJSON, data, "", "\t")
			if err != nil {
				return emperror.Wrapf(err, "cannot pretty json")
			}

			action := gitlab.CommitAction{
				Content: prettyJSON.String(),
			}
			if group.Gitlab == nil {
				action.Action = "create"
				creations++
			} else if group.Deleted {
				action.Action = "delete"
				deletions++
			} else {
				action.Action = "update"
				updates++
			}

			fname := fmt.Sprintf("%v.json", group.Id)
			action.FilePath = fname

			found, err := group.zot.gitlabCheck(fname, "master")
			if err != nil {
				return emperror.Wrapf(err, "cannot check gitlab for %v", fname)
			}

			if !found {
				switch action.Action {
				case "delete":
					action.Action = ""
				case "update":
					action.Action = "create"
				}
			} else {
				switch action.Action {
				case "create":
					action.Action = "update"
				}
			}

			if action.Action != "" {
				gaction = append(gaction, &action)
			}
		}
		gcommit := fmt.Sprintf("#%v/%v machine sync creation:%v / deletion:%v / update:%v  at %v",
			i+1, slices, creations, deletions, updates, synctime.String())
		opt := gitlab.CreateCommitOptions{
			Branch:        &gbranch,
			CommitMessage: &gcommit,
			StartBranch:   nil,
			Actions:       gaction,
			AuthorEmail:   nil,
			AuthorName:    nil,
		}
		zot.logger.Infof("committing groups %v to %v of %v to gitlab", start, end, num)
		_, _, err := zot.git.Commits.CreateCommit(zot.gitProject.ID, &opt)
		if err != nil {
			// thats very bad. let's try with the single file method and update fallback
			zot.logger.Errorf("error committing to gitlab. fallback to single Group commit: %v", err)
			for _, group := range parts {
				if _, err := group.uploadGitlab(); err != nil {
					return emperror.Wrapf(err, "cannot upload Group %v", group.Id)
				}
			}
			//return emperror.Wrapf(err, "cannot commit")
		}
		sqlstr = fmt.Sprintf("UPDATE %s.groups SET gitlab=$1 WHERE id=$2", zot.dbSchema)
		for _, group := range parts {
			t := sql.NullTime{
				Time:  synctime,
				Valid: !group.Deleted,
			}
			params := []interface{}{
				t,
				group.Id,
			}
			resp, err := zot.db.Exec(sqlstr, params...)
			if err != nil {
				return emperror.Wrapf(err, "cannot update gitlab sync time for %v", group.Id)
			}
			zot.logger.Debugf("%v", resp)
		}
	}
	return nil
}
