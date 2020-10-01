package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/goph/emperror"
	"github.com/op/go-logging"
	"gitlab.fhnw.ch/hgk-dima/zotero-sync/pkg/zotero"
	"strings"
	"time"
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
	Objectid        int64
	Status          string
	Deleted         int
	Modficationdate time.Time
	Modifier        string
	Metadata        map[string][]Value
	form            *Form
}

func (o *Object) GetRightHolders() ([]string, error) {
	sqlstr := "SELECT r.rightholder FROM zotmedia.rights r, upload.files f, zotmedia.master m WHERE r.masterid=m.masterid AND f.masterid=m.masterid AND f.objectid=?"
	o.form.log.Infof("query: %s", sqlstr)
	rows2, err := o.form.sourceDB.Query(sqlstr, o.Objectid)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot execute query %s", sqlstr)
	}
	defer rows2.Close()
	rhs := []string{}
	for rows2.Next() {
		var rh string
		if err := rows2.Scan(&rh); err != nil {
			return nil, emperror.Wrap(err, "cannot scan data")
		}
		found := false
		for _, r := range rhs {
			if r == rh {
				found = true
				break
			}
		}
		if !found {
			rhs = append(rhs, rh)
		}
	}
	return rhs, nil
}

func (o *Object) IterateFiles(callback func(f *File) error) error {
	sqlstr := "SELECT m.masterid, c.name, m.signature, m.type, m.mimetype FROM upload.files f, zotmedia.master m, zotmedia.collection c WHERE m.collectionid=c.collectionid AND f.masterid=m.masterid AND f.objectid=?"
	o.form.log.Infof("query: %s", sqlstr)
	rows2, err := o.form.sourceDB.Query(sqlstr, o.Objectid)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute query %s", sqlstr)
	}
	defer rows2.Close()
	for rows2.Next() {
		var masterid int64
		var collection, signature, _type, mimetype string
		if err := rows2.Scan(&masterid, &collection, &signature, &_type, &mimetype); err != nil {
			return emperror.Wrapf(err, "cannot scan values")
		}
		f := &File{
			Masterid:   masterid,
			Collection: collection,
			Signature:  signature,
			Type:       _type,
			Mimetype:   mimetype,
		}

		sqlstr := "SELECT `rightholder`, `usage`, `license`, `restrictedlicense`, `access`, `label`, `modifier` FROM zotmedia.rights WHERE masterid=?"
		// o.form.log.Infof("query: %s", sqlstr)
		rows3, err := o.form.sourceDB.Query(sqlstr, f.Masterid)
		if err != nil {
			return emperror.Wrapf(err, "cannot execute query %s", sqlstr)
		}
		defer rows3.Close()
		for rows3.Next() {
			var rightholder, usage, license, restrictedlicense, access, label, modifier string
			if err := rows3.Scan(&rightholder, &usage, &license, &restrictedlicense, &access, &label, &modifier); err != nil {
				return emperror.Wrapf(err, "cannot scan rights data")
			}
			f.Note = fmt.Sprintf(
				"Rightholder: %s<br />\n"+
					"Usage: %s<br />\n"+
					"License: %s<br />\n"+
					"RestrictedLicense: %s<br />\n"+
					"Access: %s<br />\n"+
					"Label: %s<br />\n"+
					"Modifier: %s\n",
				rightholder, usage, license, restrictedlicense, access, label, modifier)
		}

		if err := callback(f); err != nil {
			return err
		}
	}
	return nil
}

type Form struct {
	sourceDB *sql.DB
	zotero   *zotero.Zotero
	log      *logging.Logger
}

func (f *Form) GetTypes() (map[int64]int64, error) {
	var t2zot = map[int64]int64{}
	sqlstr := "SELECT typeid, name, zoterogroup FROM upload.`type` WHERE zoterogroup IS NOT NULL ORDER BY typeid ASC"
	f.log.Infof("query: %s", sqlstr)
	rows, err := f.sourceDB.Query(sqlstr)
	if err != nil {
		return nil, emperror.Wrapf(err, "cannot exeute query %s", sqlstr)
	}
	defer rows.Close()
	for rows.Next() {
		var typeid int64
		var name string
		var zoterogroup sql.NullInt64
		if err := rows.Scan(&typeid, &name, &zoterogroup); err != nil {
			return nil, emperror.Wrap(err, "cannot scan content")

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
	rows2, err := f.sourceDB.Query(sqlstr, typeid)
	if err != nil {
		return emperror.Wrapf(err, "cannot execute query %s", sqlstr)
	}
	defer rows2.Close()
	for rows2.Next() {
		var objectid int64
		var status string
		var deleted int
		var modficationdate sql.NullTime
		var modifier string
		if err := rows2.Scan(&objectid, &status, &deleted, &modficationdate, &modifier); err != nil {
			return emperror.Wrapf(err, "cannot scan content")
		}
		obj := &Object{
			Objectid:        objectid,
			Status:          status,
			Deleted:         deleted,
			Modficationdate: modficationdate.Time,
			Modifier:        modifier,
			Metadata:        map[string][]Value{},
			form:            f,
		}
		sqlstr = "SELECT fieldtype, name, json FROM upload.`field` WHERE objectid=?"
		f.log.Infof("query: %s - %v", sqlstr, objectid)
		rows3, err := f.sourceDB.Query(sqlstr, objectid)
		if err != nil {
			return emperror.Wrapf(err, "cannot execute query %s", sqlstr)
		}
		defer rows3.Close()
		for rows3.Next() {
			var fieldtype string
			var name string
			var jsonstr string
			if err := rows3.Scan(&fieldtype, &name, &jsonstr); err != nil {
				return emperror.Wrap(err, "cannot scan content")
			}
			name = strings.ToLower(name)
			var data Value
			if err := json.Unmarshal([]byte(jsonstr), &data); err != nil {
				return emperror.Wrapf(err, "cannot unmarshal json - %s", jsonstr)
			}
			if _, ok := obj.Metadata[name]; !ok {
				obj.Metadata[name] = []Value{}
			}
			obj.Metadata[name] = append(obj.Metadata[name], data)
		}
		if err := callback(obj); err != nil {
			return emperror.Wrap(err, "error in callback")
		}
	}
	return nil
}
