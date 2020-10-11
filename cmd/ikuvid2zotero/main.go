package main

import (
	"database/sql"
	"flag"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	"github.com/je4/zsync/pkg/filesystem"
	"github.com/je4/zsync/pkg/zotero"
	_ "github.com/lib/pq"
	"github.com/op/go-logging"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"
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

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	// get location of config file
	cfgfile := flag.String("cfg", "/etc/mediasrv2.toml", "location of config file")
	flag.Parse()
	config := LoadConfig(*cfgfile)

	// create logger instance
	logger, lf := CreateLogger("memostream", config.Logfile, config.Loglevel)
	defer lf.Close()

	// get database connection handle
	sourceDB, err := sql.Open(config.IKUVidDB.ServerType, config.IKUVidDB.DSN)
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

	grp, err := zot.LoadGroupLocal(zoterogroup)
	if err != nil {
		fmt.Errorf("cannot load group #%v - %v", zoterogroup, err)
		return
	}

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
		//"SELECT masterid, signature, masterurl, width, height, duration FROM zotmedia.fullcachewithurl WHERE collection_id=? AND `type`=? AND parentid IS NULL AND signature LIKE ?"
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

		item, err := grp.GetItemByOldidLocal(fmt.Sprintf("%v", nr))
		if err != nil {
			fmt.Errorf("cannot load item by oldid #%v - %v -- %v", zoterogroup, nr, err)
			break
		}

		//itemData := zotero.ItemFilm{}
		itemData := zotero.ItemGeneric{}
		itemData.ItemType = "film"
		itemData.Relations = make(map[string]zotero.ZoteroStringList)
		itemData.Creators = []zotero.ItemDataPerson{}
		itemData.Tags = []zotero.ItemTag{}

		itemData.Title = titel1.String
		if titel2.Valid {
			itemData.Title += " - " + titel2.String
		}
		itemData.Date = jahr.String
		if regie.Valid {
			ps := strings.Split(regie.String, ",")
			for _, p := range ps {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				var person zotero.ItemDataPerson
				p2s := strings.Split(p, " ")
				if len(p2s) == 2 {
					person = zotero.ItemDataPerson{
						CreatorType: "director",
						FirstName:   strings.TrimSpace(p2s[0]),
						LastName:    strings.TrimSpace(p2s[1]),
					}
				} else {
					person = zotero.ItemDataPerson{
						CreatorType: "director",
						FirstName:   "",
						LastName:    strings.TrimSpace(p),
					}
				}
				itemData.Creators = append(itemData.Creators, person)
			}
		}

		if kategorie.Valid {
			ks := strings.Split(kategorie.String, " ")
			for _, k := range ks {
				k = strings.TrimSpace(k)
				if k == "" {
					continue
				}
				itemData.Tags = append(itemData.Tags, zotero.ItemTag{
					Tag: k,
				})
			}
		}
		if stichwort.Valid {
			itemData.Tags = append(itemData.Tags, zotero.ItemTag{
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

		itemMeta := zotero.ItemMeta{}

		if item == nil {
			item, err = grp.CreateItemLocal(&itemData, &itemMeta, fmt.Sprintf("%v", nr))
			if err != nil {
				fmt.Errorf("cannot create item #%v - %v -- %v", zoterogroup, nr, err)
				break
			}
		} else {
			item.Data = itemData
			item.Data.Version = item.Version
			//item.Meta = itemMeta
			item.Status = zotero.SyncStatus_Modified
			item.Data.Key = item.Key
			item.UpdateLocal()
		}

		logger.Infof("%v", item)

		if tech.Valid || medium.Valid {
			technote, err := grp.GetItemByOldidLocal(fmt.Sprintf("%v.tech", nr))
			if err != nil {
				fmt.Errorf("cannot load item by oldid #%v - %v.tech -- %v", zoterogroup, nr, err)
				break
			}

			//techItemData := zotero.ItemDataNote{}
			techItemData := zotero.ItemGeneric{}
			techItemData.ItemType = "note"
			techItemData.Relations = make(map[string]zotero.ZoteroStringList)
			techItemData.Tags = []zotero.ItemTag{}
			techItemData.Collections = []string{}
			techItemData.ParentItem = zotero.Parent(item.Key)
			//			techItemData.Title = "Technical Information"

			if tech.Valid {
				techItemData.Note += strings.Replace(tech.String, "\n", "<br />\n", -1) + "<br />\n"
			}
			if medium.Valid {
				techItemData.Note += "Medium: " + medium.String
			}

			technoteMeta := zotero.ItemMeta{}

			if technote == nil {
				technote, err = grp.CreateItemLocal(&techItemData, &technoteMeta, fmt.Sprintf("%v.tech", nr))
				if err != nil {
					fmt.Errorf("cannot create item #%v - %v.tech -- %v", zoterogroup, nr, err)
					break
				}
			} else {
				technote.Data = techItemData
				technote.Data.Version = technote.Version
				//item.Meta = itemMeta
				technote.Status = zotero.SyncStatus_Modified
				technote.Data.Key = technote.Key
				technote.UpdateLocal()
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
				fmt.Errorf("cannot scan result data: %v", err)
				break
			}
			oldid := fmt.Sprintf("%v-%v", nr, masterid)
			item2, err := grp.GetItemByOldidLocal(fmt.Sprintf("%v", oldid))
			if err != nil {
				fmt.Errorf("cannot load item by oldid #%v - %v: %v", zoterogroup, oldid, err)
				break
			}

			itemData := zotero.ItemGeneric{}
			// itemData := zotero.ItemDataAttachment{}
			itemData.ItemType = "attachment"
			itemData.LinkMode = "linked_url"
			itemData.Relations = make(map[string]zotero.ZoteroStringList)
			itemData.Creators = []zotero.ItemDataPerson{}
			itemData.Tags = []zotero.ItemTag{}
			// wird schon klappen....
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
			itemData.ParentItem = zotero.Parent(item.Key)

			itemMeta := zotero.ItemMeta{}

			if item2 == nil {
				item2, err = grp.CreateItemLocal(&itemData, &itemMeta, oldid)
				if err != nil {
					fmt.Errorf("cannot create item #%v.%v - %v", zoterogroup, oldid, err)
					break
				}
			} else {
				item2.Data = itemData
				item2.Data.Version = item2.Version
				//item.Meta = itemMeta
				item2.Status = zotero.SyncStatus_Modified
				item2.Data.Key = item2.Key
				item2.UpdateLocal()
			}
		}

	}
}
