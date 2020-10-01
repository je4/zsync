package zotmedia

import (
	"database/sql"
	"fmt"
	"github.com/goph/emperror"
	"github.com/op/go-logging"
	"net/url"
	"regexp"
)

type Mediaserver struct {
	db                *sql.DB
	dbSchema          string
	logger            *logging.Logger
	base              *url.URL
	MediaserverRegexp *regexp.Regexp
}

func NewMediaserver(mediaserverbase string, db *sql.DB, dbSchema string, logger *logging.Logger) (*Mediaserver, error) {
	url, err := url.Parse(mediaserverbase)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot parse url %s", mediaserverbase)
	}
	rs := fmt.Sprintf("%s/([^/]+)/([^/]+)/.*")
	regexp, err := regexp.Compile(rs)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot compile regexp %s", rs)
	}
	return &Mediaserver{
		db:                db,
		dbSchema:          dbSchema,
		logger:            logger,
		base:              url,
		MediaserverRegexp: regexp,
	}, nil
}
