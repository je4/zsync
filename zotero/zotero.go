package zotero

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/goph/emperror"
	"github.com/op/go-logging"
	"gopkg.in/resty.v1"
	"net/http"
	"net/url"
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
func (zot *Zotero) CheckWait(header http.Header) bool {
	var err error
	backoff := int64(0)
	retryAfter := int64(0)
	backoffStr := header.Get("Backoff")
	if backoffStr != "" {
		backoff, err = strconv.ParseInt(backoffStr, 10, 64)
		if err != nil {
			backoff = 0
		}
	}
	retryAfterStr := header.Get("Retry-After")
	if retryAfterStr != "" {
		backoff, err = strconv.ParseInt(retryAfterStr, 10, 64)
		if err != nil {
			retryAfter = 0
		}
	}

	sleep := backoff
	if retryAfter > sleep {
		sleep = retryAfter
	}
	if sleep > 0 {
		zot.logger.Infof("Sleeping %v seconds (Backoff: %v / RetryAfter: %v)", sleep, backoff, retryAfter)
		time.Sleep(time.Duration(sleep) * time.Second)
	}
	return retryAfter > 0
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
	resp, err := zot.client.R().
		SetHeader("Accept", "application/json").
		Get(endpoint)
	if err != nil {
		zot.logger.Panicf("cannot execute rest call to %s", endpoint)
	}
	rawBody := resp.Body()
	types := []itemType{}
	if err := json.Unmarshal(rawBody, &types); err != nil {
		zot.logger.Panicf("cannot unmarshal %s: %v", string(rawBody), err)
	}
	zot.CheckWait(resp.Header())
	str += fmt.Sprintln("switch item.(type) {")
	for _, _type := range types {
		str += fmt.Sprintf("case Item%s:\n", strings.Title(_type.ItemType))
	}

	for _, _type := range types {
		str += fmt.Sprintf("type Item%s struct {\n", strings.Title(_type.ItemType))
		str += fmt.Sprintln("	ItemDataBase")
		str += fmt.Sprintln("	Creators        []ItemDataPerson `json:\"creators\"`")

		endpoint = "/itemTypeFields"
		resp, err := zot.client.R().
			SetHeader("Accept", "application/json").
			SetQueryParam("itemType", _type.ItemType).
			Get(endpoint)
		if err != nil {
			zot.logger.Panicf("cannot execute rest call to %s", endpoint)
		}
		rawBody := resp.Body()
		fields := []field{}
		if err := json.Unmarshal(rawBody, &fields); err != nil {
			zot.logger.Panicf("cannot unmarshal %s: %v", string(rawBody), err)
		}
		zot.CheckWait(resp.Header())
		for _, field := range fields {
			str += fmt.Sprintf("   %s string `json:\"%s,omitempty\"` // %s\n", strings.Title(field.Field), field.Field, field.Localized)
		}
		str += fmt.Sprintln("}")
		str += fmt.Sprintln("")
	}
	return
}

func (zot *Zotero) GetGroup(groupId int64) (*Group, error) {
	endpoint := fmt.Sprintf("/groups/%v", groupId)
	zot.logger.Infof("rest call: %s", endpoint)

	resp, err := zot.client.R().
		SetHeader("Accept", "application/json").
		Get(endpoint)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot get current key from %s", endpoint)
	}
	rawBody := resp.Body()
	group := &Group{}
	if err := json.Unmarshal(rawBody, group); err != nil {
		return nil, emperror.Wrapf(err, "cannot unmarshal %s", string(rawBody))
	}
	zot.CheckWait(resp.Header())
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
