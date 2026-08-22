package model

import (
	"encoding/json/v2"
	"fmt"
	"reflect"
	"strconv"

	"emperror.dev/errors"
)

type Library struct {
	Type  string `json:"type"`
	Id    int64  `json:"id"`
	Name  string `json:"name"`
	Links any    `json:"links"`
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

func (res *ItemCollectionCreateResult) CheckSuccess(id int64) (string, error) {
	successlist := res.GetSuccess()
	success, ok := successlist[id]
	if ok {
		return success, nil
	}
	unchangedlist := res.GetUnchanged()
	unchanged, ok := unchangedlist[id]
	if ok {
		return unchanged, nil
	}
	failed := res.GetFailed()
	fail, ok := failed[id]
	if ok {
		return fail.Key, errors.New(fmt.Sprintf("item %v update/creation failed with [%v]%v", fail.Key, fail.Code, fail.Message))
	}
	return "", errors.New(fmt.Sprintf("invalid id %v", id))
}

func (res *ItemCollectionCreateResult) GetSuccess() map[int64]string {
	result := map[int64]string{}
	for key, val := range res.Success {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			continue
		}
		result[id] = val
	}
	return result
}

func (res *ItemCollectionCreateResult) GetUnchanged() map[int64]string {
	result := map[int64]string{}
	for key, val := range res.Unchanged {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			continue
		}
		result[id] = val
	}
	return result
}

func (res *ItemCollectionCreateResult) GetFailed() map[int64]ItemCollectionCreateResultFailed {
	result := map[int64]ItemCollectionCreateResultFailed{}
	for key, val := range res.Failed {
		id, err := strconv.ParseInt(key, 10, 64)
		if err != nil {
			continue
		}
		result[id] = val
	}
	return result
}

// RelationList are empty array or string map
type RelationList map[string]string

func (rl RelationList) MarshalJSON() ([]byte, error) {
	if rl == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(map[string]string(rl))
}

func (rl *RelationList) UnmarshalJSON(data []byte) error {
	var i any
	if err := json.Unmarshal(data, &i); err != nil {
		return err
	}
	switch d := i.(type) {
	case map[string]any:
		*rl = RelationList{}
		for key, val := range d {
			(*rl)[key], _ = val.(string)
		}
	case []any:
		if len(d) > 0 {
			return errors.New(fmt.Sprintf("invalid object list for type RelationList - %s", string(data)))
		}
		*rl = RelationList{}
	}
	return nil
}

// ZoteroStringList - zotero returns single item lists as string
type ZoteroStringList []string

func (irl *ZoteroStringList) UnmarshalJSON(data []byte) error {
	var i any
	if err := json.Unmarshal(data, &i); err != nil {
		return err
	}
	switch i.(type) {
	case string:
		*irl = ZoteroStringList{i.(string)}
	case []any:
		*irl = ZoteroStringList{}
		for _, i2 := range i.([]any) {
			str, ok := i2.(string)
			if !ok {
				return errors.New(fmt.Sprintf("invalid type %v for %v", reflect.TypeOf(i2), i2))
			}
			*irl = append(*irl, str)
		}
	default:
		return errors.New(fmt.Sprintf("invalid type %v for %v", reflect.TypeOf(i), string(data)))
	}
	return nil
}

// ParentCollection - zotero treats empty strings as false in ParentCollection
type ParentCollection string

func (pc ParentCollection) MarshalJSON() ([]byte, error) {
	if pc == "" {
		return []byte("false"), nil
	}
	return json.Marshal(string(pc))
}

func (pc *ParentCollection) UnmarshalJSON(data []byte) error {
	var i any
	if err := json.Unmarshal(data, &i); err != nil {
		return err
	}
	switch v := i.(type) {
	case bool:
		*pc = ""
	case string:
		*pc = ParentCollection(v)
	default:
		return errors.New(fmt.Sprintf("invalid type %v for %v", reflect.TypeOf(i), string(data)))
	}
	return nil
}

type Parent = ParentCollection
