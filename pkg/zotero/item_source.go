package zotero

import (
	"fmt"
	"github.com/vanng822/go-solr/solr"
	"gitlab.fhnw.ch/hgk-dima/zotero-sync/pkg/zotmedia"
	"gitlab.fhnw.ch/mediathek/search/gsearch/pkg/gsearch"
	"html/template"
	"reflect"
	"regexp"
	"strings"
)

/* *******************************
Functions of gsearch.Source interface
******************************* */

var zoteroIgnoreMetaFields = []string{
	"AbstractNote",
	"Collections",
	"Creators",
	"Date",
	"Media",
	"Place",
	"Relations",
	"Tags",
	"Title",
	"Extra",
	"Note",
}

func (item *Item) Name() string {
	return "zotero2"
}

func (item *Item) GetTitle() string {
	return item.Data.Title

}

func (item *Item) GetPlace() string {
	return item.Data.Place

}

func (item *Item) GetDate() string {
	return item.Data.Date

}

func (item *Item) GetCollectionTitle() string {
	return item.Group.Data.Name

}

func (item *Item) GetPersons() []gsearch.Person {
	var persons []gsearch.Person
	for _, c := range item.Data.Creators {
		name := strings.Trim(fmt.Sprintf("%s, %s", c.LastName, c.FirstName), " ,")
		if name != "" {
			persons = append(persons, gsearch.Person{
				Name: name,
				Role: c.CreatorType,
			})
		}
	}
	return persons
}

// name:value
var zoteroTagVariable = regexp.MustCompile(`^(acl_meta|acl_content):(.+)$`)

func (item *Item) GetACL() map[string][]string {
	meta := Text2Metadata(item.Group.Data.Description)
	meta2 := Text2Metadata(item.Data.AbstractNote)
	for key, val := range meta2 {
		meta[key] = val
	}
	acls := make(map[string][]string)
	for key, val := range meta {
		if strings.Index(key, "acl_") == 0 {
			if _, ok := acls[key]; !ok {
				acls[key] = []string{}
			}
			acls[key] = val
		}
	}
	return acls
}

func (item *Item) GetCatalogs() []string {
	meta := Text2Metadata(item.Group.Data.Description)
	meta2 := Text2Metadata(item.Data.AbstractNote)
	for key, val := range meta2 {
		meta[key] = val
	}
	catalogs := []string{}
	for key, val := range meta {
		if strings.Index(key, "catalog") == 0 {
			catalogs = append(catalogs, val...)
		}
	}
	return catalogs
}

func (item *Item) GetCategories() []string {
	categories := []string{}
	for _, collection := range item.Data.Collections {
		coll, err := item.Group.GetCollectionByKeyLocal(collection)
		if err != nil {
			continue
		}
		cat := coll.Data.Name
		for coll.Data.ParentCollection != "" {
			coll, err = item.Group.GetCollectionByKeyLocal(string(coll.Data.ParentCollection))
			if err != nil {
				break
			}
			cat = fmt.Sprintf("%v!!%v", coll.Data.Name, cat)
		}
		categories = append(categories, cat)
	}
	return categories
}

func (item *Item) GetTags() []string {
	var tags []string
	for _, t := range item.Data.Tags {
		// ignore variables (i.e. <name>:<value>
		if !zoteroTagVariable.MatchString(t.Tag) {
			tags = AppendIfMissing(tags, strings.ToLower(t.Tag))
		}
	}
	tags = AppendIfMissing(tags, strings.ToLower(item.Group.Data.Name))

	for _, c := range item.Data.Collections {
		for _, collKey := range item.Data.Collections {
			coll, err := item.Group.GetCollectionByKeyLocal(collKey)
			if err != nil {
				item.Group.Zot.logger.Errorf("could not load collection #%v.%v", item.Group.Data.Id, collKey)
				continue
			}
			if coll.Key == c {
				tags = AppendIfMissing(tags, strings.ToLower(coll.Data.Name))
				for ok := true; ok; ok = (coll.Data.ParentCollection == "") {
					coll2, err := item.Group.GetCollectionByKeyLocal(string(coll.Data.ParentCollection))
					if err != nil {
						break
					}
					tags = AppendIfMissing(tags, strings.ToLower(coll2.Data.Name))
				}
			}
		}
	}
	return tags
}

func (item *Item) GetMedia(ms *zotmedia.MediaserverMySQL) map[string]gsearch.MediaList {
	medias := make(map[string]gsearch.MediaList)
	//var types []string
	children, err := item.getChildrenLocal()
	if err != nil {
		return medias
	}
	for _, child := range *children {
		if child.Data.ItemType != "attachment" {
			continue
		}
		var collection, signature string
		if child.Data.LinkMode == "linked_url" || child.Data.LinkMode == "imported_url" {
			// check for mediaserver url
			var ok bool
			collection, signature, ok = ms.IsMediaserverURL(child.Data.Url)
			if !ok {
				// if not, create mediaserver entry
				collection = fmt.Sprintf("zotero_%v", item.Group.Id)
				signature = fmt.Sprintf("%v.%v_url", item.Group.Id, child.Key)
				if err := ms.CreateMasterUrl(collection, signature, child.Data.Url); err != nil {
					item.Group.Zot.logger.Errorf("cannot create mediaserver entry for item #%v.%s %s/%s",
						item.Group.Id,
						child.Key,
						collection,
						signature)
					continue
				}
			}
		} else {
			collection = fmt.Sprintf("zotero_%v", item.Group.Id)
			signature = fmt.Sprintf("%v.%v_enclosure", item.Group.Id, child.Key)
			folder, err := item.Group.GetFolder()
			if err != nil {
				item.Group.Zot.logger.Errorf("cannot get folder of attachment file: %v", err)
				continue
			}
			filepath := fmt.Sprintf("%s/%s", folder, child.Key)
			found, err := item.Group.Zot.fs.FileExists(folder, child.Key)
			if err != nil {
				item.Group.Zot.logger.Errorf("cannot check existence of file %s: %v", filepath, err)
				continue
			}
			if !found {
				item.Group.Zot.logger.Warningf("file %s does not exist", filepath)
				continue
			}
			url := fmt.Sprintf("%s/%s", item.Group.Zot.fs.Protocol(), filepath)
			if err := ms.CreateMasterUrl(collection, signature, url); err != nil {
				item.Group.Zot.logger.Errorf("cannot create mediaserver entry for item #%s.%s %s/%s",
					item.Group.Id,
					item.Key,
					collection,
					signature)
				continue
			}
		}

		if collection != "" && signature != "" {
			metadata, err := ms.GetMetadata(collection, signature)
			if err != nil {
				item.Group.Zot.logger.Errorf("cannot get metadata for %s/%s", collection, signature)
				continue
			}
			name := child.Data.Title
			if name == "" {
				name = fmt.Sprintf("#%v.%v", item.Group.Id, child.Key)
			}
			media := gsearch.Media{
				Name:     name,
				Mimetype: metadata.Mimetype,
				Type:     metadata.Type,
				Uri:      fmt.Sprintf("mediaserver:%s/%s", collection, signature),
				Width:    metadata.Width,
				Height:   metadata.Height,
				Duration: metadata.Duration,
			}
			if _, ok := medias[media.Type]; !ok {
				medias[media.Type] = []gsearch.Media{}
			}
			medias[media.Type] = append(medias[media.Type], media)
		}
	}
	return medias
}

var mediaserverRegexp = regexp.MustCompile("^mediaserver:([^/]+)/(.+)$")

func (item *Item) GetPoster(ms *zotmedia.MediaserverMySQL) *gsearch.Media {
	medias := item.GetMedia(ms)
	if _, ok := medias["video"]; ok {
		if len(medias["video"]) > 0 {
			vid := medias["video"][0]
			if matches := mediaserverRegexp.FindStringSubmatch(vid.Uri); matches != nil {
				collection := matches[1]
				signature := fmt.Sprintf("%s$$timeshot03", matches[2])
				metadata, err := ms.GetMetadata(collection, signature)
				if err == nil {
					return &gsearch.Media{
						Name:     "poster",
						Mimetype: metadata.Mimetype,
						Type:     metadata.Type,
						Uri:      fmt.Sprintf("mediaserver:%v/%v", collection, signature),
						Width:    metadata.Width,
						Height:   metadata.Height,
						Duration: metadata.Duration,
					}
				}
			}
		}
	}
	if _, ok := medias["image"]; ok {
		for _, media := range medias["image"] {
			if strings.ToLower(media.Name) == "poster" {
				return &media
			}
		}
		if len(medias["image"]) > 0 {
			return &(medias["image"][0])
		}
	}
	return nil
}

func (item *Item) GetNotes() []gsearch.Note {
	notes := []gsearch.Note{}
	note := strings.TrimSpace(item.Data.Note)
	if note != "" {
		notes = append(notes, gsearch.Note{
			Title: item.Data.Title,
			Note:  template.HTML(note),
		})
	}

	children, err := item.getChildrenLocal()
	if err != nil {
		item.Group.Zot.logger.Errorf("cannot load children of #%v.%v", item.Group.Id, item.Key)
		return notes
	}
	for _, child := range *children {
		note := strings.TrimSpace(child.Data.Note)
		if note == "" {
			continue
		}
		notes = append(notes, gsearch.Note{
			Title: child.Data.Title,
			Note:  template.HTML(note),
		})
	}
	return notes
}

func (item *Item) GetAbstract() string {
	return TextNoMeta(item.Data.AbstractNote + "\n" + item.Data.Extra)
}

var zoterolinkregexp = regexp.MustCompile("^https?://zotero.org/groups/([^/]+)/items/([^/]+)$")

func (item *Item) GetReferences() []gsearch.Reference {
	var references []gsearch.Reference
	for key, values := range item.Data.ItemDataBase.Relations {
		for _, value := range values {
			if matches := zoterolinkregexp.FindStringSubmatch(value); matches != nil {
				signature := fmt.Sprintf("zotero-%s.%s", matches[1], matches[2])
				references = append(references, gsearch.Reference{
					Type:      key,
					Signature: signature,
				})
			}
		}
	}
	return references
}

func (item *Item) GetMeta() map[string]string {
	var result = make(map[string]string)
	s := reflect.ValueOf(&item.Data).Elem()
	typeOfT := s.Type()
	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		fname := typeOfT.Field(i).Name
		if fname == "ItemDataBase" {
			continue
		}
		valstr := strings.TrimSpace(fmt.Sprintf("%v", f.Interface()))
		if valstr != "" {
			result[fname] = valstr
		}
	}
	s = reflect.ValueOf(&item.Data.ItemDataBase).Elem()
	typeOfT = s.Type()
	for i := 0; i < s.NumField(); i++ {
		f := s.Field(i)
		fname := typeOfT.Field(i).Name
		valstr := strings.TrimSpace(fmt.Sprintf("%v", f.Interface()))
		if valstr != "" {
			result[fname] = valstr
		}
	}
	return result
}

func (item *Item) GetExtra() map[string]string {
	var result = make(map[string]string)
	for key, val := range item.GetMeta() {
		if gsearch.InList(zoteroIgnoreMetaFields, key) {
			continue
		}
		result[key] = val
	}
	return result
}

func (item *Item) GetContentType() string {
	am := strings.TrimSpace(item.Data.ArtworkMedium)
	if am != "" {
		return strings.ToLower(am)
	}
	pt := strings.TrimSpace(item.Data.PresentationType)
	if pt != "" {
		return strings.ToLower(pt)
	}

	return strings.ToLower(item.Data.ItemDataBase.ItemType)
}

func (item *Item) GetQueries() []gsearch.Query {
	return nil
}

func (item *Item) GetSolrDoc() *solr.Document {
	return nil
}

func (item *Item) GetContentString() string {
	return ""

}

func (item *Item) GetContentMime() string {
	return ""

}
