package main

import (
	"database/sql"
	"encoding/json/v2"
	"strings"
	"time"

	"emperror.dev/errors"
	"github.com/je4/zsync/v2/pkg/zotero/storage"
	"github.com/op/go-logging"
)

type Value struct {
	Type  string
	Value interface{}
}

type File struct {
	Masterid   int64
	Collection string
	Signature  string
	Mimetype   string
	Type       string
	Note       string
}

type Object struct {
	Objectid         int64
	Status           string
	Deleted          bool
	Modificationdate time.Time
	Modifier         string
	Metadata         map[string][]Value
}

type Form struct {
	sourceDB *sql.DB
	storage  *storage.Storage
	log      *logging.Logger
}

func (f *Form) GetTypes() (map[int64]int64, error) {
	var t2zot = map[int64]int64{}
	sqlstr := "SELECT typeid, name, zoterogroup FROM upload.`type` WHERE zoterogroup IS NOT NULL ORDER BY typeid ASC"
	f.log.Infof("query: %s", sqlstr)
	rows, err := f.sourceDB.Query(sqlstr)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot execute query %s", sqlstr)
	}
	defer rows.Close()
	for rows.Next() {
		var typeid int64
		var name string
		var zoterogroup sql.NullInt64
		if err := rows.Scan(&typeid, &name, &zoterogroup); err != nil {
			return nil, errors.Wrap(err, "cannot scan content")
		}
		if !zoterogroup.Valid {
			continue
		}
		f.log.Infof("%s[%v] - %v", name, typeid, zoterogroup.Int64)
		t2zot[typeid] = zoterogroup.Int64
	}
	return t2zot, nil
}

func (f *Form) IterateObjects(typeid, zoterogroup int64, callback func(obj *Object) error) error {
	sqlstr := "SELECT objectid, status, deleted, modificationdate, modifier FROM upload.object WHERE typeid=?"
	f.log.Infof("query: %s", sqlstr)
	rows, err := f.sourceDB.Query(sqlstr, typeid)
	if err != nil {
		return errors.Wrapf(err, "cannot execute query %s", sqlstr)
	}
	defer rows.Close()
	for rows.Next() {
		obj := &Object{
			Metadata: map[string][]Value{},
		}
		var modifier sql.NullString
		var moddate sql.NullString
		if err := rows.Scan(&obj.Objectid, &obj.Status, &obj.Deleted, &moddate, &modifier); err != nil {
			return errors.Wrap(err, "cannot scan content")
		}
		if modifier.Valid {
			obj.Modifier = modifier.String
		}
		if moddate.Valid {
			obj.Modificationdate, _ = time.Parse("2006-01-02 15:04:05", moddate.String)
		}
		sqlstr2 := "SELECT fieldtype, name, `json` FROM upload.metadata WHERE objectid=?"
		rows3, err := f.sourceDB.Query(sqlstr2, obj.Objectid)
		if err != nil {
			return errors.Wrapf(err, "cannot execute query %s", sqlstr2)
		}
		defer rows3.Close()
		for rows3.Next() {
			var fieldtype string
			var name string
			var jsonstr string
			if err := rows3.Scan(&fieldtype, &name, &jsonstr); err != nil {
				return errors.Wrap(err, "cannot scan content")
			}
			name = strings.ToLower(name)
			var data Value
			if err := json.Unmarshal([]byte(jsonstr), &data); err != nil {
				return errors.Wrapf(err, "cannot unmarshal json - %s", jsonstr)
			}
			if _, ok := obj.Metadata[name]; !ok {
				obj.Metadata[name] = []Value{}
			}
			obj.Metadata[name] = append(obj.Metadata[name], data)
		}
		if err := callback(obj); err != nil {
			return errors.Wrap(err, "error in callback")
		}
	}
	return nil
}
