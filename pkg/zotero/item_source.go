package zotero

import (
	"fmt"
	"github.com/vanng822/go-solr/solr"
	"gitlab.fhnw.ch/hgk-dima/zotero-sync/pkg/zotmedia"
	"gitlab.fhnw.ch/mediathek/search/gsearch/pkg/gsearch"
	"regexp"
	"strings"
)

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

func (item *Item) GetMedia(ms *zotmedia.Mediaserver) map[string]gsearch.MediaList {
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
		if child.Data.LinkMode == "linked_url" {
			if matches := ms.MediaserverRegexp.FindStringSubmatch(child.Data.Url); matches != nil {

			}
		}
	}
	return medias
	/*
			meta := child.Data.Media.Metadata
			t := strings.ToLower(meta.Type)
			// empty type == no media
			if t == "" {
				if strings.HasSuffix(child.Data.Url, ".mp4") {
					t = "video"
					meta.Mimetype = "video/mp4"
				} else {
					continue
				}
			}
			// if type not in list create it
			if _, ok := zot.medias[t]; !ok {
				zot.medias[t] = MediaList{}
				types = append(types, t)
			}
			zot.medias[t] = append(zot.medias[t], Media{
				Name:     child.Data.Title,
				Mimetype: meta.Mimetype,
				Type:     t,
				Uri:      child.Data.Url,
				Width:    int64(meta.Width),
				Height:   int64(meta.Height),
				Duration: int64(meta.Duration),
			})
		}
		// sort medias according to their name
		for _, t := range types {
			sort.Sort(zot.medias[t])
		}
		return zot.medias

	*/
}

func (item *Item) GetPoster() *gsearch.Media {
	return nil
}

func (item *Item) GetNotes() []gsearch.Note {
	return nil
}

func (item *Item) GetAbstract() string {
	return ""

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
