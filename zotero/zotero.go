package zotero

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/goph/emperror"
	"github.com/op/go-logging"
	"gopkg.in/resty.v1"
	"net/url"
)

type Zotero struct {
	baseUrl *url.URL
	apiKey  string
	client  *resty.Client
	logger  *logging.Logger
	db      *sql.DB
	dbSchema string
}

func NewZotero(baseUrl string, apiKey string, db *sql.DB, dbSchema string, logger *logging.Logger) (*Zotero, error) {
	burl, err := url.Parse(baseUrl)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot create url from %s", baseUrl)
	}
	zot := &Zotero{baseUrl: burl,
		apiKey: apiKey,
		logger: logger,
		db:db,
		dbSchema:dbSchema,
	}
	zot.Init()
	return zot, nil
}

func (zot *Zotero) Init() {
	zot.client = resty.New()
	zot.client.SetHostURL(zot.baseUrl.String())
	zot.client.SetAuthToken(zot.apiKey)
	zot.client.SetContentLength(true)
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

