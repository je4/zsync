package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/je4/zsync/v2/pkg/zotero/model"
	"github.com/je4/zsync/v2/pkg/zotero/storage"
	"github.com/op/go-logging"
	"github.com/rs/zerolog"
)

type logger struct {
	handle *os.File
}

var zoterogroup int64 = 2571475

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

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// get location of config file
	cfgfile := flag.String("cfg", "/etc/mediasrv2.toml", "location of config file")
	flag.Parse()
	config := LoadConfig(*cfgfile)

	// create logger instance
	logger, lf := CreateLogger("ikuvid2zotero", config.Logfile, config.Loglevel)
	defer lf.Close()

	// get database connection handle
	sourceDB, err := sql.Open(config.IKUVidDB.ServerType, config.IKUVidDB.DSN)
	if err != nil {
		log.Fatalf("error opening database: %v", err)
	}
	defer sourceDB.Close()

	// Open doesn't open a connection. Validate DSN data:
	err = sourceDB.Ping()
	if err != nil {
		log.Fatalf("error pinging database: %v", err)
	}

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

	mediasqlstr := "select " +
		"	m.masterid, " +
		"	m.signature, " +
		"	concat('https://ba14ns21403-sec1.fhnw.ch/mediasrv/',`col`.`name`,'/',`m`.`signature`,'/master') AS `masterurl`," +
		"	c.width, c.height, c.duration" +
		" FROM master m, collection col, cache c" +
		" WHERE m.collectionid=col.collectionid " +
		"	AND c.masterid=m.masterid " +
		"	AND c.action='master'" +
		"	AND m.collectionid=?" +
		"	AND m.`type`=?" +
		"	AND m.signature=?"

	getMediaStmt, err := sourceDB.Prepare(mediasqlstr)
	if err != nil {
		logger.Errorf("cannot prepare statement %s - %v", mediasqlstr, err)
		return
	}

	sqlstr := "SELECT `Archiv-Nr`, `Kategorie`, `Stichwort`, `Titel1`, `Titel2`," +
		" `Autor Regie`, `Land`, `Produktionsjahr`, `Dauer`, `Bemerkungen`, `Techn Daten`," +
		" `Videothek`, `Aufnahmedatum`, `Sender`, `Originalsprache`, `Sprache Untertitel`," +
		" `Sprachen 2-Kanal`, `Medium`, `S W Bemerkungen`, `Titel und Bemerkungen`," +
		" `Techn Hinweise` FROM rfid.`source_ikuvid`"
	rows, err := sourceDB.Query(sqlstr)
	if err != nil {
		logger.Errorf("cannot execute query %s - %v", sqlstr, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var nr int64
		var kategorie, stichwort, titel1, titel2, regie, land, jahr, dauer, bemerkungen,
			tech, videothek, aufnahme, sender, originalsprache, sparache, sprache2kanal, medium,
			farbe, titelbemerkung, techhinweise sql.NullString

		if err := rows.Scan(&nr, &kategorie, &stichwort, &titel1, &titel2, &regie, &land, &jahr, &dauer, &bemerkungen,
			&tech, &videothek, &aufnahme, &sender, &originalsprache, &sparache, &sprache2kanal, &medium,
			&farbe, &titelbemerkung, &techhinweise); err != nil {
			logger.Errorf("error scanning values - %v", err)
			return
		}

		item, err := zotStorage.GetItemByOldid(zoterogroup, fmt.Sprintf("%v", nr))
		if err != nil {
			fmt.Printf("cannot load item by oldid #%v - %v: %v\n", zoterogroup, nr, err)
			break
		}

		itemData := model.ItemGeneric{}
		itemData.ItemType = "film"
		itemData.Relations = make(map[string]model.ZoteroStringList)
		itemData.Creators = []model.ItemDataPerson{}
		itemData.Tags = []model.ItemTag{}
		itemData.Collections = []string{}

		if regie.Valid {
			itemData.Creators = append(itemData.Creators, model.ItemDataPerson{
				CreatorType: "director",
				Name:        regie.String,
			})
		}
		if titel1.Valid {
			itemData.Title = strings.TrimSpace(titel1.String)
		}
		if titel2.Valid {
			if itemData.Title != "" {
				itemData.Title += " - "
			}
			itemData.Title += strings.TrimSpace(titel2.String)
		}
		if land.Valid {
			itemData.SetString("country", strings.TrimSpace(land.String))
		}
		if dauer.Valid {
			itemData.SetString("runningTime", strings.TrimSpace(dauer.String))
		}
		if jahr.Valid {
			itemData.Date = strings.TrimSpace(jahr.String)
		}
		if originalsprache.Valid {
			itemData.SetString("language", strings.TrimSpace(originalsprache.String))
		}
		if medium.Valid {
			itemData.SetString("videoRecordingFormat", strings.TrimSpace(medium.String))
		}
		if videothek.Valid {
			itemData.SetString("archiveLocation", strings.TrimSpace(videothek.String))
		}
		if kategorie.Valid {
			kat := strings.Split(kategorie.String, ";")
			for _, k := range kat {
				k = strings.TrimSpace(k)
				if k == "" {
					continue
				}
				itemData.Tags = append(itemData.Tags, model.ItemTag{
					Tag: k,
				})
				coll, err := zotStorage.GetCollectionByName(zoterogroup, k, "")
				if err != nil {
					fmt.Printf("cannot load collection %s: %v\n", k, err)
					break
				}
				if coll == nil {
					logger.Infof("creating collection %v", k)
					coll, err = zotStorage.CreateCollection(zoterogroup, &model.CollectionData{
						Key:              model.CreateKey(),
						Name:             k,
						Version:          0,
						Relations:        model.RelationList{},
						ParentCollection: "",
					})
					if err != nil {
						fmt.Printf("cannot create collection %s: %v\n", k, err)
						break
					}
				}
				itemData.Collections = append(itemData.Collections, coll.Key)
			}
		}
		if stichwort.Valid {
			itemData.Tags = append(itemData.Tags, model.ItemTag{
				Tag: stichwort.String,
			})
		}

		if bemerkungen.Valid {
			itemData.AbstractNote += bemerkungen.String + "\n"
		}
		if sender.Valid {
			itemData.AbstractNote += "Aufnahme: " + sender.String
			if aufnahme.Valid {
				itemData.AbstractNote += ", " + aufnahme.String
			}
			itemData.AbstractNote += "\n"
		}

		itemMeta := model.ItemMeta{}

		if item == nil {
			item, err = zotStorage.CreateItem(zoterogroup, &itemData, &itemMeta, fmt.Sprintf("%v", nr))
			if err != nil {
				fmt.Printf("cannot create item #%v - %v -- %v\n", zoterogroup, nr, err)
				break
			}
		} else {
			item.Data = itemData
			item.Data.Version = item.Version
			item.Status = model.SyncStatus_Modified
			item.Data.Key = item.Key
			if err := zotStorage.UpdateItem(zoterogroup, item); err != nil {
				fmt.Printf("cannot update item %v: %v\n", item.Key, err)
			}
		}

		params := []interface{}{
			64,
			"video",
			fmt.Sprintf("%04d.h264_1100k.mp4", nr),
		}
		rows2, err := getMediaStmt.Query(params...)
		if err != nil {
			logger.Errorf("cannot execute query %s / %v - %v", mediasqlstr, params, err)
			return
		}
		defer rows2.Close()
		for rows2.Next() {
			var masterid, width, height, duration int64
			var masterurl, vsig string
			if err := rows2.Scan(&masterid, &vsig, &masterurl, &width, &height, &duration); err != nil {
				fmt.Printf("cannot scan result data: %v\n", err)
				break
			}
			oldid := fmt.Sprintf("%v-%v", nr, masterid)
			item2, err := zotStorage.GetItemByOldid(zoterogroup, fmt.Sprintf("%v", oldid))
			if err != nil {
				fmt.Printf("cannot load item by oldid #%v - %v: %v\n", zoterogroup, oldid, err)
				break
			}

			itemData := model.ItemGeneric{}
			itemData.ItemType = "attachment"
			itemData.LinkMode = "linked_url"
			itemData.Relations = make(map[string]model.ZoteroStringList)
			itemData.Creators = []model.ItemDataPerson{}
			itemData.Tags = []model.ItemTag{}
			d, _ := time.ParseDuration(fmt.Sprintf("%vs", duration))
			d = d.Round(time.Minute)
			h := d / time.Hour
			d -= h * time.Hour
			m := d / time.Minute
			d -= m * time.Minute
			s := d / time.Second
			itemData.Note = fmt.Sprintf("Resolution: %vx%v<br />\nDuration: %d:%02d:%02d", width, height, h, m, s)

			itemData.Title = vsig
			itemData.Url = masterurl
			itemData.ParentItem = item.Key

			itemMeta := model.ItemMeta{}

			if item2 == nil {
				item2, err = zotStorage.CreateItem(zoterogroup, &itemData, &itemMeta, oldid)
				if err != nil {
					fmt.Printf("cannot create item #%v.%v - %v\n", zoterogroup, oldid, err)
					break
				}
			} else {
				item2.Data = itemData
				item2.Data.Version = item2.Version
				item2.Status = model.SyncStatus_Modified
				item2.Data.Key = item2.Key
				if err := zotStorage.UpdateItem(zoterogroup, item2); err != nil {
					fmt.Printf("cannot update item %v: %v\n", item2.Key, err)
				}
			}
		}
	}
}
