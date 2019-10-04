package zotero

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/goph/emperror"
	"github.com/lib/pq"
	"github.com/op/go-logging"
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
	Success   map[string]string                           `json:"success"`
	Unchanged map[string]string                           `json:"unchanged"`
	Failed    map[string]ItemCollectionCreateResultFailed `json:"failed"`
}

type Deletions struct {
	Collections []string `json:"collections"`
	Searches    []string `json:"searches"`
	Items       []string `json:"items"`
	Tags        []string `json:"tags"`
	Settings    []string `json:"settings"`
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

func NewZotero(baseUrl string, apiKey string, db *sql.DB, dbSchema string, attachmentFolder string, newGroupActive bool, logger *logging.Logger) (*Zotero, error) {
	burl, err := url.Parse(baseUrl)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot create url from %s", baseUrl)
	}
	zot := &Zotero{
		baseUrl:          burl,
		apiKey:           apiKey,
		logger:           logger,
		db:               db,
		dbSchema:         dbSchema,
		attachmentFolder: attachmentFolder,
		newGroupActive:   newGroupActive,
	}
	err = zot.Init()
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot init zotero")
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
