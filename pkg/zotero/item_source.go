package zotero

import (
	"fmt"
	"github.com/vanng822/go-solr/solr"
	"gitlab.fhnw.ch/hgk-dima/zotero-sync/pkg/zotmedia"
	"gitlab.fhnw.ch/mediathek/search/gsearch/pkg/gsearch"
	"regexp"
	"strings"
)

/* *******************************
Functions of gsearch.Source interface
******************************* */

func (item *Item) Name() string {
	return "zotero"
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

func (item *Item) GetPoster() *gsearch.Media {
	return nil
}

func (item *Item) GetNotes() []gsearch.Note {
	return nil
}

func (item *Item) GetAbstract() string {
	return TextNoMeta(item.Data.AbstractNote + "\n" + item.Data.Extra)
}

func (item *Item) GetReferences() []gsearch.Reference {
	return nil
}

func (item *Item) GetMeta() map[string]string {
	return nil
}

func (item *Item) GetExtra() map[string]string {
	return nil
}

func (item *Item) GetContentType() string {
	return ""
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
