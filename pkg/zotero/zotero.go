package zotero

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/goph/emperror"
	"github.com/je4/zsync/pkg/filesystem"
	"github.com/lib/pq"
	"github.com/op/go-logging"
	"gopkg.in/resty.v1"
	"net/http"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var regexpTextVariables = regexp.MustCompile(`([a-zA-Z0-9_]+:([^ ` + "\n" + `<"]+|"[^"]+"))`)
var regexpRemoveEmpty = regexp.MustCompile(`(?m)^\s*$[\r\n]*|[\r\n]+\s+\z`)

func Text2Metadata(str string) map[string][]string {
	meta := map[string][]string{}
	if slices := regexpTextVariables.FindAllString(str, -1); slices != nil {
		for _, slice := range slices {
			kv := strings.Split(slice, ":")
			if len(kv) != 2 {
				continue
			}
			if _, ok := meta[kv[0]]; !ok {
				meta[kv[0]] = []string{}
			}
			meta[kv[0]] = append(meta[kv[0]], strings.TrimSpace(strings.Trim(kv[1], ` "`)))
		}
	}
	return meta
}

func TextNoMeta(str string) string {
	h := regexpTextVariables.ReplaceAllString(str, " ")
	h = regexpRemoveEmpty.ReplaceAllString(h, "")
	return h
}

type Zotero struct {
	baseUrl  *url.URL
	apiKey   string
	client   *resty.Client
	Logger   *logging.Logger
	db       *sql.DB
	dbSchema string
	//attachmentFolder string
	newGroupActive bool
	CurrentKey     *ApiKey
	Fs             filesystem.FileSystem
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

func NewZotero(baseUrl string, apiKey string, db *sql.DB, fs filesystem.FileSystem, dbSchema string, newGroupActive bool, logger *logging.Logger, dbOnly bool) (*Zotero, error) {
	burl, err := url.Parse(baseUrl)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot create url from %s", baseUrl)
	}
	zot := &Zotero{
		baseUrl:  burl,
		apiKey:   apiKey,
		Logger:   logger,
		db:       db,
		Fs:       fs,
		dbSchema: dbSchema,
		//attachmentFolder: attachmentFolder,
		newGroupActive: newGroupActive,
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
		zot.Logger.Infof("Sleeping %v seconds (RetryAfter)", retryAfter)
		time.Sleep(time.Duration(retryAfter) * time.Second)
	}
	return retryAfter > 0
}

func (zot *Zotero) GetFS() filesystem.FileSystem {
	return zot.Fs
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
		zot.Logger.Infof("Sleeping %v seconds (Backoff)", backoff)
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
			zot.Logger.Panicf("cannot execute rest call to %s", endpoint)
		}
		if zot.CheckRetry(resp.Header()) {
			break
		}
	}
	rawBody := resp.Body()
	types := []itemType{}
	if err := json.Unmarshal(rawBody, &types); err != nil {
		zot.Logger.Panicf("cannot unmarshal %s: %v", string(rawBody), err)
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
				zot.Logger.Panicf("cannot execute rest call to %s", endpoint)
			}
			if !zot.CheckRetry(resp.Header()) {
				break
			}
		}
		rawBody := resp.Body()
		fields := []field{}
		if err := json.Unmarshal(rawBody, &fields); err != nil {
			zot.Logger.Panicf("cannot unmarshal %s: %v", string(rawBody), err)
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
	zot.Logger.Infof("rest call: %s", endpoint)

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
	group.Init()
	zot.CheckBackoff(resp.Header())
	group.Zot = zot
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

	group.Init()
	return &group, nil
}

func (zot *Zotero) DeleteUnknownGroupsLocal(knownGroups []int64) error {
	placeHolder := []string{}
	params := []interface{}{}
	for i := 0; i < len(knownGroups); i++ {
		placeHolder = append(placeHolder, fmt.Sprintf("$%v", i+1))
		params = append(params, sql.NullInt64{
			Int64: knownGroups[i],
			Valid: true,
		})
	}
	sqlstr := fmt.Sprintf("UPDATE %s.groups SET deleted=true WHERE id NOT IN (%s)", zot.dbSchema, strings.Join(placeHolder, ", "))
	_, err := zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, knownGroups)
	}
	sqlstr = fmt.Sprintf("UPDATE %s.syncgroups SET active=false WHERE id NOT IN (%s)", zot.dbSchema, strings.Join(placeHolder, ", "))
	_, err = zot.db.Exec(sqlstr, params...)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, knownGroups)
	}
	return nil
}

func (zot *Zotero) CreateEmptyGroupLocal(groupId int64) (bool, SyncDirection, error) {
	active := false
	direction := SyncDirection_BothLocal
	sqlstr := fmt.Sprintf("INSERT INTO %s.groups (id,version,created,modified) VALUES($1, 0, NOW(), NOW())", zot.dbSchema)
	_, err := zot.db.Exec(sqlstr, groupId)
	if err != nil {
		return false, SyncDirection_None, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, groupId)
	}
	sqlstr = fmt.Sprintf("INSERT INTO %s.syncgroups(id,active,direction) VALUES($1, $2, $3)", zot.dbSchema)
	params := []interface{}{
		groupId,
		active,
		SyncDirectionString[direction],
	}
	_, err = zot.db.Exec(sqlstr, params...)
	if err != nil {
		// existiert schon
		if IsUniqueViolation(err, "syncgroups_pkey") {
			var dirstr string
			sqlstr := fmt.Sprintf("SELECT active, direction FROM %s.syncgroups WHERE id=$1", zot.dbSchema)
			if err := zot.db.QueryRow(sqlstr, groupId).Scan(&active, &dirstr); err != nil {
				return false, SyncDirection_None, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, groupId)
			}
			direction = SyncDirectionId[dirstr]
		} else {
			return false, SyncDirection_None, emperror.Wrapf(err, "cannot execute %s: %v", sqlstr, params)
		}
	}
	return active, direction, nil
}

func (zot *Zotero) LoadGroupLocal(groupId int64) (*Group, error) {
	group := &Group{
		Id:      groupId,
		Version: 0,
		Links:   nil,
		Meta:    GroupMeta{},
		Data:    GroupData{},
		Active:  zot.newGroupActive,
		Zot:     zot,
	}
	zot.Logger.Debugf("loading Group #%v from database", groupId)
	sqlstr := fmt.Sprintf("SELECT version, created, modified, data, active, direction, tags,"+
		" itemversion, collectionversion, tagversion, gitlab"+
		" FROM %s.groups g, %s.syncgroups sg WHERE g.id=sg.id AND g.id=$1", zot.dbSchema, zot.dbSchema)
	row := zot.db.QueryRow(sqlstr, groupId)
	var jsonstr sql.NullString
	var directionstr string
	var gitlab sql.NullTime
	err := row.Scan(&group.Version,
		&group.Meta.Created,
		&group.Meta.LastModified,
		&jsonstr,
		&group.Active,
		&directionstr,
		&group.syncTags,
		&group.ItemVersion,
		&group.CollectionVersion,
		&group.TagVersion,
		&gitlab)
	if err != nil {
		// error real error
		if err != sql.ErrNoRows {
			return nil, emperror.Wrapf(err, "error scanning result of %s: %v", sqlstr, groupId)
		}
		// just no data
		active, direction, err := zot.CreateEmptyGroupLocal(groupId)
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot create empty Group %v", groupId)
		}
		group.Active = active
		group.direction = direction
	} else {
		if gitlab.Valid {
			group.Gitlab = &gitlab.Time
		}
		group.direction = SyncDirectionId[directionstr]
	}
	if jsonstr.Valid {
		err = json.Unmarshal([]byte(jsonstr.String), &group.Data)
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot unmarshall Group data %s", jsonstr)
		}
	}
	group.Init()
	return group, nil
}

func (zot *Zotero) LoadGroupsLocal() ([]*Group, error) {
	zot.Logger.Debugf("loading Groups from database")
	sqlstr := fmt.Sprintf("SELECT id FROM %s.syncgroups sg WHERE sg.active=true", zot.dbSchema)
	rows, err := zot.db.Query(sqlstr)
	if err != nil {
		return nil, emperror.Wrapf(err, "error executing sql query: %v", sqlstr)
	}
	defer rows.Close()
	grps := []*Group{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, emperror.Wrap(err, "cannot scan row")
		}
		grp, err := zot.LoadGroupLocal(id)
		if err != nil {
			zot.Logger.Errorf("error loading Group #%v: %v", id, err)
			continue
		}
		zot.Logger.Infof("Group #%v - %v loaded", grp.Id, grp.Data.Name)
		grps = append(grps, grp)
	}

	return grps, nil
}
