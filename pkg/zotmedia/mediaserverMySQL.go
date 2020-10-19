package zotmedia

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/go-sql-driver/mysql"
	"github.com/goph/emperror"
	"github.com/op/go-logging"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type Collection struct {
	CollectionId,
	EstateId int64
	Name,
	Description string
	StorageId int64
	JWTKey    string
}

type Metadata struct {
	Mimetype string
	Type     string
	Ext      string
	Sha256   string
	Width    int64
	Height   int64
	Duration int64
	Filesize int64
	Image    interface{}
	Exif     interface{}
	Video    interface{}
}

type MediaserverMySQL struct {
	db                *sql.DB
	dbSchema          string
	logger            *logging.Logger
	base              *url.URL
	mediaserverRegexp *regexp.Regexp
	collections       []*Collection
}

func NewJWT(secret string, subject string, valid int64) (tokenString string, err error) {
	exp := time.Now().Unix() + valid
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": strings.ToLower(subject),
		"exp": exp,
	})
	log.Println("NewJWT( ", secret, ", ", subject, ", ", exp)
	tokenString, err = token.SignedString([]byte(secret))
	return tokenString, err
}

func NewMediaserverMySQL(mediaserverbase string, db *sql.DB, dbSchema string, logger *logging.Logger) (*MediaserverMySQL, error) {
	url, err := url.Parse(mediaserverbase)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot parse url %s", mediaserverbase)
	}
	rs := fmt.Sprintf("%s/([^/]+)/([^/]+)/.*")
	regexp, err := regexp.Compile(rs)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot compile regexp %s", rs)
	}
	ms := &MediaserverMySQL{
		db:                db,
		dbSchema:          dbSchema,
		logger:            logger,
		base:              url,
		mediaserverRegexp: regexp,
	}
	ms.Init()
	return ms, nil
}

func (ms *MediaserverMySQL) Init() error {
	ms.collections = []*Collection{}
	sqlstr := fmt.Sprintf("SELECT c.collectionid, c.estateid, c.name, c.description, c.storageid, s.jwtkey"+
		" FROM %s.collection c, %s.storage s"+
		" WHERE c.storageid=s.storageid", ms.dbSchema, ms.dbSchema)
	rows, err := ms.db.Query(sqlstr)
	if err != nil {
		return emperror.Wrapf(err, "cannot load collections - %s", sqlstr)
	}
	defer rows.Close()
	var jwtkey sql.NullString
	for rows.Next() {
		coll := &Collection{}
		if err := rows.Scan(
			&coll.CollectionId,
			&coll.EstateId,
			&coll.Name,
			&coll.Description,
			&coll.StorageId,
			&jwtkey); err != nil {
			return emperror.Wrap(err, "cannot scan values of collections")
		}
		coll.JWTKey = jwtkey.String
		ms.collections = append(ms.collections, coll)
	}
	return nil
}

func (ms *MediaserverMySQL) IsMediaserverURL(url string) (string, string, bool) {
	if matches := ms.mediaserverRegexp.FindStringSubmatch(url); matches != nil {
		return matches[1], matches[2], true
	}
	return "", "", false
}

func (ms *MediaserverMySQL) GetCollectionByName(name string) (*Collection, error) {
	for _, coll := range ms.collections {
		if coll.Name == name {
			return coll, nil
		}
	}
	return nil, fmt.Errorf("cannot find collection %s", name)
}

func (ms *MediaserverMySQL) GetCollectionById(id int64) (*Collection, error) {
	for _, coll := range ms.collections {
		if coll.CollectionId == id {
			return coll, nil
		}
	}
	return nil, fmt.Errorf("cannot find collection #%v", id)
}

func (ms *MediaserverMySQL) CreateMasterUrl(collection, signature, url string) error {
	coll, err := ms.GetCollectionByName(collection)
	if err != nil {
		return emperror.Wrap(err, "cannot get collection")
	}
	sqlstr := fmt.Sprintf("INSERT INTO %s.master(collectionid,signature,urn) VALUES(?, ?, ?)", ms.dbSchema)
	params := []interface{}{coll.CollectionId, signature, url}
	_, err = ms.db.Exec(sqlstr, params...)
	if err != nil {
		myerr, ok := err.(*mysql.MySQLError)
		// no mysql error
		if !ok {
			return emperror.Wrapf(err, "cannot create master: %s - %v", sqlstr, params)
		}
		// mysql error 1062: duplicate entry
		if myerr.Number != 1062 {
			return emperror.Wrapf(myerr, "cannot create master: %s - %v", sqlstr, params)
		} else {
			ms.logger.Infof("master #%s/%s already in database", collection, signature)
		}
	}
	return nil
}

func (ms *MediaserverMySQL) GetMetadata(collection, signature string) (*Metadata, error) {

	coll, err := ms.GetCollectionByName(collection)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot find collection %s", collection)
	}

	url := fmt.Sprintf("%s/%s/%s/metadata", ms.base, collection, signature)
	if coll.JWTKey != "" {
		// todo: test this
		sub := strings.ToLower(fmt.Sprintf("mediaserver:%s/%s/metadata", collection, signature))
		key, err := NewJWT(coll.JWTKey, sub, 1800)
		if err != nil {
			return nil, emperror.Wrapf(err, "cannot build jwt token")
		}
		url += fmt.Sprintf("?token=%s", key)
	}
	resp, err := http.Get(url)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot load url %s", url)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, resp.Body); err != nil {
		return nil, emperror.Wrapf(err, "cannot read result data from url %s", url)
	}
	result := &Metadata{}
	if err := json.Unmarshal(buf.Bytes(), result); err != nil {
		return nil, emperror.Wrapf(err, "cannot unmarshal metadata for %s - %s", url, string(buf.Bytes()))
	}
	return result, nil
}
