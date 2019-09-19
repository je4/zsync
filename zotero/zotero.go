package zotero

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/goph/emperror"
	"github.com/op/go-logging"
	"gopkg.in/resty.v1"
	"net/url"
	"strings"
)

type Zotero struct {
	baseUrl          *url.URL
	apiKey           string
	client           *resty.Client
	logger           *logging.Logger
	db               *sql.DB
	dbSchema         string
	attachmentFolder string
}

func NewZotero(baseUrl string, apiKey string, db *sql.DB, dbSchema string, attachmentFolder string, logger *logging.Logger) (*Zotero, error) {
	burl, err := url.Parse(baseUrl)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot create url from %s", baseUrl)
	}
	zot := &Zotero{baseUrl: burl,
		apiKey:           apiKey,
		logger:           logger,
		db:               db,
		dbSchema:         dbSchema,
		attachmentFolder: attachmentFolder,
	}
	zot.Init()
	return zot, nil
}

func (zot *Zotero) Init() {
	zot.client = resty.New()
	zot.client.SetHostURL(zot.baseUrl.String())
	zot.client.SetAuthToken(zot.apiKey)
	zot.client.SetContentLength(true)
	zot.client.SetRedirectPolicy(resty.FlexibleRedirectPolicy(3))
}

func (zot *Zotero) PrintTypeStructs() {
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
	for _, _type := range types {
		fmt.Printf("type Item%s struct {\n", strings.Title(_type.ItemType))
		fmt.Println("	ItemDataBase")
		fmt.Println("	Creators        []ItemDataPerson `json:\"creators\"`")

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
		for _, field := range fields {
			fmt.Printf("   %s string `json:\"%s,omitempty\"` // %s\n", strings.Title(field.Field), field.Field, field.Localized)
		}
		fmt.Println("}")
		fmt.Println("")
	}
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
	group.zot = zot
	return group, nil
}
