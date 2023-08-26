package main

import (
	"database/sql"
	"emperror.dev/errors"
	"flag"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/je4/zsync/v2/pkg/filesystem"
	"github.com/je4/zsync/v2/pkg/zotero"
	_ "github.com/lib/pq"
	"github.com/op/go-logging"
	"log"
	"math/rand"
	"os"
	"regexp"
	"strings"
	"time"
)

type logger struct {
	handle *os.File
}

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
		//defer lf.Close()

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
	for key, val := range vals {
		if valstr, ok := val.Value.(string); ok {
			if valstr == oldval {
				continue
			}
			valstr = strings.TrimSpace(valstr)
			if valstr == "" {
				continue
			}
			if key > 0 {
				str += kit
			}
			str += valstr
			oldval = valstr
		}
	}
	return str
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// get location of config file
	cfgfile := flag.String("cfg", "/etc/form2zotero.toml", "location of config file")
	flag.Parse()
	config := LoadConfig(*cfgfile)

	// create logger instance
	logger, lf := CreateLogger("memostream", config.Logfile, config.Loglevel)
	defer lf.Close()

	// get database connection handle
	sourceDB, err := sql.Open(config.FormDB.ServerType, config.FormDB.DSN)
	if err != nil {
		panic(err.Error())
	}
	defer sourceDB.Close()

	// Open doesn't open a connection. Validate DSN data:
	err = sourceDB.Ping()
	if err != nil {
		panic(err.Error())
	}

	// get database connection handle
	zoteroDB, err := sql.Open(config.ZoteroDB.ServerType, config.ZoteroDB.DSN)
	if err != nil {
		panic(err.Error())
	}
	defer zoteroDB.Close()

	// Open doesn't open a connection. Validate DSN data:
	err = zoteroDB.Ping()
	if err != nil {
		panic(err.Error())
	}

	fs, err := filesystem.NewS3Fs(config.S3.Endpoint, config.S3.AccessKeyId, config.S3.SecretAccessKey, config.S3.UseSSL)
	if err != nil {
		log.Fatalf("cannot conntct to s3 instance: %v", err)
	}

	rand.Seed(time.Now().Unix())

	zot, err := zotero.NewZotero(config.Endpoint, config.Apikey, zoteroDB, fs, config.ZoteroDB.Schema, false, logger, false)
	if err != nil {
		logger.Errorf("cannot create zotero instance: %v", err)
		return
	}

	form := &Form{
		sourceDB: sourceDB,
		zotero:   zot,
		log:      logger,
	}

	t2zot, err := form.GetTypes()
	if err != nil {
		logger.Errorf("%v", err)
		return
	}

	logger.Infof("%v", t2zot)
	logger.Infof("%v", zot)

	for key, zoterogroup := range t2zot {
		form.IterateObjects(key, zoterogroup, func(obj *Object) error {
			logger.Infof("%v", obj)
			for key, vals := range obj.Metadata {
				form.log.Infof("%s: %v", key, vals)
			}

			grp, err := form.zotero.LoadGroupLocal(zoterogroup)
			if err != nil {
				return errors.Wrapf(err, "cannot load group #%v", zoterogroup)
			}
			item, err := grp.GetItemByOldidLocal(fmt.Sprintf("%v", obj.Objectid))
			if err != nil {
				return errors.Wrapf(err, "cannot load item by oldid #%v - %v", zoterogroup, obj.Objectid)
			}

			//			itemData := zotero.ItemDocument{}
			itemData := zotero.ItemGeneric{}
			itemData.ItemType = "document"
			itemData.Relations = make(map[string]zotero.ZoteroStringList)
			itemData.Creators = []zotero.ItemDataPerson{}

			if titles, ok := obj.Metadata["title"]; ok {
				itemData.Title = meta2string(titles, " - ")
			}
			if abstracts, ok := obj.Metadata["abstract"]; ok {
				itemData.AbstractNote = meta2string(abstracts, "; ")
			}
			if projectends, ok := obj.Metadata["projectend"]; ok {
				itemData.Date = meta2string(projectends, "; ")
			}

			var university, institute string
			if universities, ok := obj.Metadata["university"]; ok {
				university = meta2string(universities, "; ")
			}
			if institutes, ok := obj.Metadata["institute"]; ok {
				institute = meta2string(institutes, "; ")
			}
			rightholders, err := obj.GetRightHolders()
			if err != nil {
				return errors.Wrapf(err, "cannot get rightholders of %v", obj.Objectid)
			}
			for _, rh := range rightholders {
				var nameregexp = regexp.MustCompile("^([^,]+),(.+)$")
				idp := zotero.ItemDataPerson{
					CreatorType: "author",
					FirstName:   "",
					LastName:    rh,
				}
				if found := nameregexp.FindStringSubmatch(rh); found != nil {
					idp.LastName = strings.TrimSpace(found[1])
					idp.FirstName = strings.TrimSpace(found[2])
				}
				itemData.Creators = append(itemData.Creators, idp)
			}

			if university != "" && institute != "" {
				itemData.Creators = append(itemData.Creators, zotero.ItemDataPerson{
					CreatorType: "contributor",
					FirstName:   institute,
					LastName:    university,
				})
			}

			itemData.Tags = []zotero.ItemTag{}
			for _, tfield := range []string{"tag", "theme", "institute", "projecttype", "institute"} {
				if tags, ok := obj.Metadata[tfield]; ok {
					tagstr := meta2string(tags, ", ")
					_tags := strings.Split(tagstr, ",")
					for _, t := range _tags {
						t = strings.TrimSpace(t)
						if t == "" {
							continue
						}
						itemData.Tags = append(itemData.Tags, zotero.ItemTag{
							Tag: t,
						})
					}
				}
			}

			itemMeta := zotero.ItemMeta{}

			if item == nil {
				item, err = grp.CreateItemLocal(&itemData, &itemMeta, fmt.Sprintf("%v", obj.Objectid))
				if err != nil {
					return errors.Wrapf(err, "cannot create item #%v - %v", zoterogroup, obj.Objectid)
				}
			} else {
				item.Data = itemData
				item.Data.Version = item.Version
				//item.Meta = itemMeta
				item.Status = zotero.SyncStatus_Modified
				item.Data.Key = item.Key
				item.UpdateLocal()
			}

			form.log.Infof("%v", item)

			if err := obj.IterateFiles(func(f *File) error {
				item2, err := grp.GetItemByOldidLocal(fmt.Sprintf("%v-%v", obj.Objectid, f.Masterid))
				if err != nil {
					return errors.Wrapf(err, "cannot load item by oldid #%v - %v-%v", zoterogroup, obj.Objectid, f.Masterid)
				}

				itemData := zotero.ItemGeneric{}
				// itemData := zotero.ItemDataAttachment{}
				itemData.ItemType = "attachment"
				itemData.LinkMode = "linked_url"
				itemData.Relations = make(map[string]zotero.ZoteroStringList)
				itemData.Creators = []zotero.ItemDataPerson{}
				itemData.Tags = []zotero.ItemTag{}
				itemData.Note = f.Note

				itemData.Title = f.Signature
				itemData.Url = fmt.Sprintf("https://ba14ns21403-sec1.fhnw.ch/mediasrv/%v/%v/master", f.Collection, f.Signature)
				itemData.ParentItem = zotero.Parent(item.Key)

				itemMeta := zotero.ItemMeta{}

				if item2 == nil {
					item2, err = grp.CreateItemLocal(&itemData, &itemMeta, fmt.Sprintf("%v-%v", obj.Objectid, f.Masterid))
					if err != nil {
						return errors.Wrapf(err, "cannot create item #%v - %v", zoterogroup, obj.Objectid)
					}
				} else {
					item2.Data = itemData
					item2.Data.Version = item2.Version
					//item.Meta = itemMeta
					item2.Status = zotero.SyncStatus_Modified
					item2.Data.Key = item2.Key
					item2.UpdateLocal()
				}

				return nil
			}); err != nil {
				return errors.Wrapf(err, "cannot iterate files of %v", obj)
			}

			return nil
		})
	}
}
