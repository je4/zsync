package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
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

var zoterogroup int64 = 2604593

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
	logger, lf := CreateLogger("vonarx2zotero", config.Logfile, config.Loglevel)
	defer lf.Close()

	// get database connection handle
	sourceDB, err := sql.Open(config.VonArxVidDB.ServerType, config.VonArxVidDB.DSN)
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
	zotStorage := storage.NewStorage(zoteroDB, config.ZoteroDB.Schema, false, &zlog)

	mediasqlstr := "select " +
		"	m.masterid, " +
		"	m.signature, " +
		"	concat('https://ba14ns21403-sec1.fhnw.ch/mediasrv/',`col`.`name`,'/',`m`.`signature`,'/master') AS `masterurl`," +
		"	c.width, c.height, c.duration" +
		" FROM mediaserver.master m, mediaserver.collection col, mediaserver.cache c" +
		" WHERE m.collectionid=col.collectionid " +
		"	AND c.masterid=m.masterid " +
		"	AND c.action='master'" +
		"	AND m.collectionid=?" +
		"	AND m.signature LIKE ?" +
		"   AND m.parentid IS NULL"

	getMediaStmt, err := sourceDB.Prepare(mediasqlstr)
	if err != nil {
		logger.Errorf("cannot prepare statement %s - %v", mediasqlstr, err)
		return
	}

	sqlstr := "SELECT FILM_NUMMER,Bezeichnung,Bewertung,Dauer_Zeit,AutorInnen,Klasse,Farbe_sw,Kamera," +
		"Aufgabenstellung,Dauer_Bezeichnung,Prodjahrbez,Produktionsjahr,Ton,Anzahl_Beispiele,Art_der_Produktion," +
		"Filmmaterial,Minuten,Rollennummer,Rollenthema,Sekunden,Verweis_auf_Publikation,Bemerkung,Dauer_Sekunden," +
		"Dozenten,Varianten,Weitere_Angaben_zur_Person" +
		" FROM rfid.source_von_arx_video "
	rows, err := sourceDB.Query(sqlstr)
	if err != nil {
		logger.Errorf("cannot execute query %s - %v", sqlstr, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var FILM_NUMMER int64
		var Bezeichnung, Bewertung, Dauer_Zeit, AutorInnen, Klasse, Farbe_sw, Kamera,
			Aufgabenstellung, Dauer_Bezeichnung, Prodjahrbez, Produktionsjahr, Ton, Anzahl_Beispiele, Art_der_Produktion,
			Filmmaterial, Minuten, Rollennummer, Rollenthema, Sekunden, Verweis_auf_Publikation, Bemerkung, Dauer_Sekunden,
			Dozenten, Varianten, Weitere_Angaben_zur_Person string

		if err := rows.Scan(&FILM_NUMMER, &Bezeichnung, &Bewertung, &Dauer_Zeit, &AutorInnen, &Klasse, &Farbe_sw, &Kamera,
			&Aufgabenstellung, &Dauer_Bezeichnung, &Prodjahrbez, &Produktionsjahr, &Ton, &Anzahl_Beispiele, &Art_der_Produktion,
			&Filmmaterial, &Minuten, &Rollennummer, &Rollenthema, &Sekunden, &Verweis_auf_Publikation, &Bemerkung, &Dauer_Sekunden,
			&Dozenten, &Varianten, &Weitere_Angaben_zur_Person); err != nil {
			logger.Errorf("error scanning values - %v", err)
			return
		}

		item, err := zotStorage.GetItemByOldid(zoterogroup, fmt.Sprintf("%v", FILM_NUMMER))
		if err != nil {
			fmt.Printf("cannot load item by oldid #%v - %v: %v\n", zoterogroup, FILM_NUMMER, err)
			break
		}

		itemData := model.ItemGeneric{}
		itemData.ItemType = "film"
		itemData.Relations = make(map[string]model.ZoteroStringList)
		itemData.Creators = []model.ItemDataPerson{}
		itemData.Tags = []model.ItemTag{}
		itemData.Collections = []string{}

		Bezeichnung = strings.TrimSpace(Bezeichnung)
		itemData.Title = Bezeichnung

		if Prodjahrbez != "" {
			itemData.Date = Prodjahrbez
		} else {
			if Produktionsjahr != "" {
				itemData.Date = Produktionsjahr
			}
		}

		sec, err := strconv.ParseInt(Dauer_Sekunden, 10, 64)
		if err == nil && sec > 0 {
			itemData.RunningTime = model.FmtDuration(time.Second * time.Duration(sec))
		}
		if Rollenthema != "" {
			itemData.Genre = Rollenthema
		}

		AutorInnen = strings.TrimSpace(AutorInnen)
		if AutorInnen != "" {
			authors := strings.SplitN(AutorInnen, ",", 2)
			for _, author := range authors {
				author := strings.TrimSpace(author)
				if author == "diverse" {
					author = "Diverse Studierende"
				}
				var person model.ItemDataPerson
				p2s := strings.Split(author, " ")
				if len(p2s) == 2 {
					person = model.ItemDataPerson{
						CreatorType: "director",
						FirstName:   strings.TrimSpace(p2s[0]),
						LastName:    strings.TrimSpace(p2s[1]),
					}
				} else {
					person = model.ItemDataPerson{
						CreatorType: "director",
						FirstName:   "",
						LastName:    strings.TrimSpace(author),
					}
				}
				itemData.Creators = append(itemData.Creators, person)
			}
		}
		Dozenten = strings.TrimSpace(Dozenten)
		if Dozenten != "" {
			authors := strings.Split(Dozenten, ",")
			for _, author := range authors {
				author := strings.TrimSpace(author)
				var person model.ItemDataPerson
				p2s := strings.SplitN(author, " ", 2)
				if len(p2s) == 2 {
					person = model.ItemDataPerson{
						CreatorType: "producer",
						FirstName:   strings.TrimSpace(p2s[0]),
						LastName:    strings.TrimSpace(p2s[1]),
					}
				} else {
					person = model.ItemDataPerson{
						CreatorType: "producer",
						FirstName:   "",
						LastName:    strings.TrimSpace(author),
					}
				}
				itemData.Creators = append(itemData.Creators, person)
			}
		}
		Kamera = strings.TrimSpace(Kamera)
		if Kamera != "" {
			authors := strings.Split(Kamera, ",")
			for _, author := range authors {
				author := strings.TrimSpace(author)
				var person model.ItemDataPerson
				p2s := strings.SplitN(author, " ", 2)
				if len(p2s) == 2 {
					person = model.ItemDataPerson{
						CreatorType: "contributor",
						FirstName:   strings.TrimSpace(p2s[0]),
						LastName:    strings.TrimSpace(p2s[1]),
					}
				} else {
					person = model.ItemDataPerson{
						CreatorType: "contributor",
						FirstName:   "",
						LastName:    strings.TrimSpace(author),
					}
				}
				itemData.Creators = append(itemData.Creators, person)
			}
		}
		Klasse = strings.TrimSpace(Klasse)
		if Klasse != "" {
			itemData.Tags = append(itemData.Tags, model.ItemTag{
				Tag: Klasse,
			})
			coll, err := zotStorage.GetCollectionByName(zoterogroup, Klasse, "")
			if err != nil {
				fmt.Printf("cannot load collection %s: %v\n", Klasse, err)
				break
			}
			if coll == nil {
				logger.Infof("creating collection %v", Klasse)
				coll, err = zotStorage.CreateCollection(zoterogroup, &model.CollectionData{
					Key:              model.CreateKey(),
					Name:             Klasse,
					Version:          0,
					Relations:        model.RelationList{},
					ParentCollection: "",
				})
				if err != nil {
					fmt.Printf("cannot create collection %s: %v\n", Klasse, err)
					break
				}
			}
			itemData.Collections = append(itemData.Collections, coll.Key)
		}

		if Aufgabenstellung != "" {
			itemData.AbstractNote += Aufgabenstellung + "\n"
		}
		if Bemerkung != "" {
			itemData.AbstractNote += Bemerkung + "\n"
		}

		itemMeta := model.ItemMeta{}

		if item == nil {
			item, err = zotStorage.CreateItem(zoterogroup, &itemData, &itemMeta, fmt.Sprintf("%v", FILM_NUMMER))
			if err != nil {
				fmt.Printf("cannot create item #%v - %v -- %v\n", zoterogroup, FILM_NUMMER, err)
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

		logger.Infof("%v", item)

		technote, err := zotStorage.GetItemByOldid(zoterogroup, fmt.Sprintf("%v.tech", FILM_NUMMER))
		if err != nil {
			fmt.Printf("cannot load item by oldid #%v - %v.tech -- %v\n", zoterogroup, FILM_NUMMER, err)
			break
		}

		techItemData := model.ItemGeneric{}
		techItemData.ItemType = "note"
		techItemData.Relations = make(map[string]model.ZoteroStringList)
		techItemData.Tags = []model.ItemTag{}
		techItemData.Collections = []string{}
		techItemData.ParentItem = item.Key

		if Farbe_sw != "" && Ton != "" {
			techItemData.Note += strings.Replace(fmt.Sprintf("%s / %s", Farbe_sw, Ton), "\n", "<br />\n", -1) + "<br />\n"
		} else {
			if Farbe_sw != "" {
				techItemData.Note += strings.Replace(Farbe_sw, "\n", "<br />\n", -1) + "<br />\n"
			}
			if Ton != "" {
				techItemData.Note += strings.Replace(Ton, "\n", "<br />\n", -1) + "<br />\n"
			}
		}
		if Kamera != "" {
			techItemData.Note += strings.Replace("Kamera: "+Kamera, "\n", "<br />\n", -1) + "<br />\n"
		}
		if Filmmaterial != "" {
			techItemData.Note += strings.Replace("Filmmaterial: "+Filmmaterial, "\n", "<br />\n", -1) + "<br />\n"
		}
		technoteMeta := model.ItemMeta{}

		if technote == nil {
			technote, err = zotStorage.CreateItem(zoterogroup, &techItemData, &technoteMeta, fmt.Sprintf("%v.tech", FILM_NUMMER))
			if err != nil {
				fmt.Printf("cannot create item #%v - %v.tech -- %v\n", zoterogroup, FILM_NUMMER, err)
				break
			}
		} else {
			technote.Data = techItemData
			technote.Data.Version = technote.Version
			technote.Status = model.SyncStatus_Modified
			technote.Data.Key = technote.Key
			if err := zotStorage.UpdateItem(zoterogroup, technote); err != nil {
				fmt.Printf("cannot update item %v: %v\n", technote.Key, err)
			}
		}

		id1 := (FILM_NUMMER - FILM_NUMMER%100) / 100
		id2 := FILM_NUMMER % 100

		params := []interface{}{
			67,
			fmt.Sprintf("%%%02d_%02d%%", id1, id2),
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
			oldid := fmt.Sprintf("%v-%v", FILM_NUMMER, masterid)
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
