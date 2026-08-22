package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"emperror.dev/errors"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/je4/zsync/v2/pkg/zotero/model"
	"github.com/je4/zsync/v2/pkg/zotero/storage"
	"github.com/op/go-logging"
	"github.com/rs/zerolog"
)

var _logformat = logging.MustStringFormatter(
	`%{time:2006-01-02T15:04:05.000} %{shortfunc} > %{level:.5s} - %{message}`,
)

func CreateLogger(module string, logfile string, loglevel string) (log *logging.Logger, lf *os.File) {
	log = logging.MustGetLogger(module)
	var err error
	if logfile != "" {
		lf, err = os.OpenFile(logfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			log.Errorf("Cannot open logfile %v: %v", logfile, err)
		}
	} else {
		lf = os.Stderr
	}
	backend := logging.NewLogBackend(lf, "", 0)
	backendLeveled := logging.AddModuleLevel(backend)
	backendLeveled.SetLevel(logging.GetLevel(loglevel), "")

	logging.SetFormatter(_logformat)
	logging.SetBackend(backendLeveled)

	return
}

func meta2string(vals []Value, kit string) string {
	str := ""
	oldval := ""
	for _, val := range vals {
		if valstr, ok := val.Value.(string); ok {
			if valstr == oldval {
				continue
			}
			valstr = strings.TrimSpace(valstr)
			if valstr == "" {
				continue
			}
			if str != "" {
				str += kit
			}
			str += valstr
			oldval = valstr
		}
	}
	return str
}

func main() {
	cfgfile := flag.String("c", "/etc/form2zotero.toml", "location of config file")
	flag.Parse()
	config := LoadConfig(*cfgfile)

	// get database connection handle
	sourceDB, err := sql.Open(config.FormDB.ServerType, config.FormDB.DSN)
	if err != nil {
		log.Fatalf("error opening database: %v", err)
	}
	defer sourceDB.Close()

	// Open doesn't open a connection. Validate DSN data:
	err = sourceDB.Ping()
	if err != nil {
		log.Fatalf("error pinging database: %v", err)
	}

	logger, lf := CreateLogger("formlist", config.Logfile, config.Loglevel)
	defer lf.Close()

	// get database connection handle
	zoteroDB, err := pgxpool.New(context.Background(), config.ZoteroDB.DSN)
	if err != nil {
		panic(err.Error())
	}
	defer zoteroDB.Close()

	// Validate DSN data:
	err = zoteroDB.Ping(context.Background())
	if err != nil {
		panic(err.Error())
	}

	zlog := zerolog.New(zerolog.ConsoleWriter{Out: os.Stderr}).With().Timestamp().Logger()
	zotStorage := storage.NewStorage(zoteroDB, false, &zlog)

	form := &Form{
		sourceDB: sourceDB,
		storage:  zotStorage,
		log:      logger,
	}

	t2zot, err := form.GetTypes()
	if err != nil {
		logger.Errorf("%v", err)
		return
	}

	logger.Infof("%v", t2zot)

	for key, zoterogroup := range t2zot {
		form.IterateObjects(key, zoterogroup, func(obj *Object) error {
			logger.Infof("%v", obj)
			for key, vals := range obj.Metadata {
				form.log.Infof("%s: %v", key, vals)
			}

			item, err := form.storage.GetItemByOldid(zoterogroup, fmt.Sprintf("%v", obj.Objectid))
			if err != nil {
				return errors.Wrapf(err, "cannot load item by oldid #%v - %v", zoterogroup, obj.Objectid)
			}

			itemData := model.ItemGeneric{}
			itemData.ItemType = "document"
			itemData.Relations = make(map[string]model.ZoteroStringList)
			itemData.Creators = []model.ItemDataPerson{}

			if titles, ok := obj.Metadata["title"]; ok {
				itemData.Title = meta2string(titles, " - ")
			}
			if abstracts, ok := obj.Metadata["abstract"]; ok {
				itemData.AbstractNote = meta2string(abstracts, "; ")
			}
			if projectends, ok := obj.Metadata["projectend"]; ok {
				itemData.Date = meta2string(projectends, "; ")
			}
			if links, ok := obj.Metadata["link"]; ok {
				itemData.Url = meta2string(links, "; ")
			}
			if shorttitles, ok := obj.Metadata["shorttitle"]; ok {
				itemData.ShortTitle = meta2string(shorttitles, "; ")
			}
			if subheadings, ok := obj.Metadata["subheading"]; ok {
				itemData.Extra = "Subheading: " + meta2string(subheadings, "\nSubheading: ")
			}
			if projectpartners, ok := obj.Metadata["projectpartner"]; ok {
				for _, pp := range projectpartners {
					itemData.Creators = append(itemData.Creators, model.ItemDataPerson{
						CreatorType: "contributor",
						Name:        fmt.Sprintf("%v", pp.Value),
					})
				}
			}
			if clientorganizations, ok := obj.Metadata["clientorganization"]; ok {
				for _, co := range clientorganizations {
					itemData.Creators = append(itemData.Creators, model.ItemDataPerson{
						CreatorType: "sponsor",
						Name:        fmt.Sprintf("%v", co.Value),
					})
				}
			}
			if leaders, ok := obj.Metadata["leader"]; ok {
				re := regexp.MustCompile(`^([^,]+), *([^,]+)$`)
				for _, l := range leaders {
					str := fmt.Sprintf("%v", l.Value)
					matches := re.FindStringSubmatch(str)
					if len(matches) == 3 {
						itemData.Creators = append(itemData.Creators, model.ItemDataPerson{
							CreatorType: "author",
							LastName:    matches[1],
							FirstName:   matches[2],
						})
					} else {
						itemData.Creators = append(itemData.Creators, model.ItemDataPerson{
							CreatorType: "author",
							Name:        str,
						})
					}
				}
			}
			if tags, ok := obj.Metadata["tag"]; ok {
				for _, tag := range tags {
					ts := strings.Split(fmt.Sprintf("%v", tag.Value), ",")
					for _, t := range ts {
						t = strings.TrimSpace(t)
						itemData.Tags = append(itemData.Tags, model.ItemTag{
							Tag: t,
						})
					}
				}
			}

			itemMeta := model.ItemMeta{}

			if item == nil {
				item, err = form.storage.CreateItem(zoterogroup, &itemData, &itemMeta, fmt.Sprintf("%v", obj.Objectid))
				if err != nil {
					return errors.Wrapf(err, "cannot create item #%v - %v", zoterogroup, obj.Objectid)
				}
			} else {
				item.Data = itemData
				item.Data.Version = item.Version
				item.Status = model.SyncStatus_Modified
				item.Data.Key = item.Key
				if err := form.storage.UpdateItem(zoterogroup, item); err != nil {
					return errors.Wrapf(err, "cannot update item %v", item.Key)
				}
			}

			form.log.Infof("%v", item)

			return nil
		})
	}
}
