package model

import (
	"encoding/json"
	"fmt"
	"time"

	"emperror.dev/errors"
)

type StringOrBool string

func (sb *StringOrBool) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		s = ""
	}
	var h StringOrBool
	h = StringOrBool(s)
	sb = &h
	return nil
}

type ItemMeta struct {
	CreatedByUser  User         `json:"createdByUser"`
	CreatorSummary string       `json:"creatorSummary,omitempty"`
	ParsedDate     StringOrBool `json:"parsedDate,omitempty"`
	NumChildren    int64        `json:"numChildren,omitempty"`
}

type ItemTag struct {
	Tag  string `json:"tag"`
	Type int64  `json:"type,omitempty"`
}

type Relations map[string]ZoteroStringList

func (rl Relations) MarshalJSON() ([]byte, error) {
	if rl == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(map[string]ZoteroStringList(rl))
}

func (rl *Relations) UnmarshalJSON(data []byte) error {
	var i any
	if err := json.Unmarshal(data, &i); err != nil {
		return err
	}
	switch d := i.(type) {
	case map[string]any:
		*rl = Relations{}
		for key, val := range d {
			var zsl ZoteroStringList
			valBytes, err := json.Marshal(val)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(valBytes, &zsl); err != nil {
				return err
			}
			(*rl)[key] = zsl
		}
	case []any:
		if len(d) > 0 {
			return errors.New(fmt.Sprintf("invalid object list for type Relations - %s", string(data)))
		}
		*rl = Relations{}
	}
	return nil
}

type ItemDataPerson struct {
	CreatorType string `json:"creatorType"`
	FirstName   string `json:"firstName,omitempty"`
	LastName    string `json:"lastName,omitempty"`
	Name        string `json:"name,omitempty"`
}

type ItemDataBase struct {
	Key          string           `json:"key,omitempty"`
	Version      int64            `json:"version,omitempty"`
	ItemType     string           `json:"itemType"`
	Tags         []ItemTag        `json:"tags"`
	Relations    Relations        `json:"relations"`
	ParentItem   string           `json:"parentItem,omitempty"`
	Collections  []string         `json:"collections,omitempty"`
	DateAdded    string           `json:"dateAdded,omitempty"`
	DateModified string           `json:"dateModified,omitempty"`
	Creators     []ItemDataPerson `json:"creators,omitempty"`
}

type Item struct {
	Key     string      `json:"key"`
	Version int64       `json:"version"`
	Library Library     `json:"library"`
	Links   any         `json:"links,omitempty"`
	Meta    ItemMeta    `json:"meta"`
	Data    ItemGeneric `json:"data"`
	Trashed bool        `json:"-"`
	Deleted bool        `json:"-"`
	Status  SyncStatus  `json:"-"`
	MD5     string      `json:"-"`
	OldId   string      `json:"-"`
	Gitlab  *time.Time  `json:"-"`
}

type ItemGitlab struct {
	LibraryId int64       `json:"libraryid"`
	Key       string      `json:"id"`
	Data      ItemGeneric `json:"data"`
	Meta      ItemMeta    `json:"meta"`
}
