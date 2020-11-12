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
	"strconv"
	"strings"
	"time"
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
	sourceDB, err := sql.Open(config.VonArxVidDB.ServerType, config.VonArxVidDB.DSN)
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
		" FROM mediaserver.master m, mediaserver.collection col, mediaserver.cache c" +
		" WHERE m.collectionid=col.collectionid " +
		"	AND c.masterid=m.masterid " +
		"	AND c.action='master'" +
		"	AND m.collectionid=?" +
		"	AND m.signature LIKE ?" +
		"   AND m.parentid IS NULL"
	//"SELECT masterid, signature, masterurl, width, height, duration FROM zotmedia.fullcachewithurl WHERE collection_id=? AND `type`=? AND parentid IS NULL AND signature LIKE ?"
	getMediaStmt, err := sourceDB.Prepare(mediasqlstr)
	if err != nil {
		logger.Errorf("cannot prepare statement %s - %v", mediasqlstr, err)
		return
	}

	sqlstr := "SELECT FILM_NUMMER,Bezeichnung,Bewertung,Dauer_Zeit,AutorInnen,Klasse,Farbe_sw,Kamera," +
		"Aufgabenstellung,Dauer_Bezeichnung,Prodjahrbez,Produktionsjahr,Ton,Anzahl_Beispiele,Art_der_Produktion," +
		"Filmmaterial,Minuten,Rollennummer,Rollenthema,Sekunden,Verweis_auf_Publikation,Bemerkung,Dauer_Sekunden," +
		"Dozenten,Varianten,Weitere_Angaben_zur_Person" +
		" FROM rfid.source_von_arx_video " +
		" WHERE Rollennummer=2"
	rows, err := sourceDB.Query(sqlstr)
	if err != nil {
		logger.Errorf("cannot execute query %s - %v", sqlstr, err)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var FILM_NUMMER, Rollennummer int64
		var Bezeichnung, Bewertung, Dauer_Zeit, AutorInnen, Klasse, Farbe_sw, Kamera, Aufgabenstellung, Dauer_Bezeichnung, Prodjahrbez, Produktionsjahr, Ton, Anzahl_Beispiele, Art_der_Produktion, Filmmaterial, Minuten, Rollenthema, Sekunden, Verweis_auf_Publikation, Bemerkung, Dauer_Sekunden, Dozenten, Varianten, Weitere_Angaben_zur_Person string

		if err := rows.Scan(
			&FILM_NUMMER,
			&Bezeichnung,
			&Bewertung,
			&Dauer_Zeit,
			&AutorInnen,
			&Klasse,
			&Farbe_sw,
			&Kamera,
			&Aufgabenstellung,
			&Dauer_Bezeichnung,
			&Prodjahrbez,
			&Produktionsjahr,
			&Ton,
			&Anzahl_Beispiele,
			&Art_der_Produktion,
			&Filmmaterial,
			&Minuten,
			&Rollennummer,
			&Rollenthema,
			&Sekunden,
			&Verweis_auf_Publikation,
			&Bemerkung,
			&Dauer_Sekunden,
			&Dozenten,
			&Varianten,
			&Weitere_Angaben_zur_Person,
		); err != nil {
			logger.Errorf("error scanning values - %v", err)
			return
		}

		item, err := grp.GetItemByOldidLocal(fmt.Sprintf("%v", FILM_NUMMER))
		if err != nil {
			fmt.Errorf("cannot load item by oldid #%v - %v -- %v", zoterogroup, FILM_NUMMER, err)
			break
		}

		//itemData := zotero.ItemFilm{}
		itemData := zotero.ItemGeneric{}
		itemData.ItemType = "film"
		itemData.Relations = make(map[string]zotero.ZoteroStringList)
		itemData.Creators = []zotero.ItemDataPerson{}
		itemData.Tags = []zotero.ItemTag{}

		itemData.Title = Bezeichnung
		itemData.Date = Produktionsjahr
		itemData.Extra = fmt.Sprintf("Rolle %02d", Rollennummer)
		sec, err := strconv.ParseInt(Dauer_Sekunden, 10, 64)
		if err == nil && sec > 0 {
			itemData.RunningTime = zotero.FmtDuration(time.Second * time.Duration(sec))
		}
		if Rollenthema != "" {
			itemData.Genre = Rollenthema
		}

		AutorInnen = strings.TrimSpace(AutorInnen)
		if AutorInnen != "" {
			authors := strings.SplitN(AutorInnen, ",", 2)
			for _, author := range authors {
				author := strings.TrimSpace(author)
				var person zotero.ItemDataPerson
				p2s := strings.Split(author, " ")
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
				var person zotero.ItemDataPerson
				p2s := strings.SplitN(author, " ", 2)
				if len(p2s) == 2 {
					person = zotero.ItemDataPerson{
						CreatorType: "producer",
						FirstName:   strings.TrimSpace(p2s[0]),
						LastName:    strings.TrimSpace(p2s[1]),
					}
				} else {
					person = zotero.ItemDataPerson{
						CreatorType: "producer",
						FirstName:   "",
						LastName:    strings.TrimSpace(author),
					}
				}
				itemData.Creators = append(itemData.Creators, person)
			}
		}

		if Aufgabenstellung != "" {
			itemData.AbstractNote += Aufgabenstellung + "\n"
		}
		if Anzahl_Beispiele != "" {
			itemData.AbstractNote += fmt.Sprintf("%s Beispiele", Anzahl_Beispiele) + "\n"
		}
		if Art_der_Produktion != "" {
			itemData.Tags = append(itemData.Tags, zotero.ItemTag{
				Tag:  Art_der_Produktion,
				Type: 0,
			})
		}
		if Bemerkung != "" {
			itemData.AbstractNote += "\n" + Bemerkung + "\n"
		}

		itemMeta := zotero.ItemMeta{}

		if item == nil {
			item, err = grp.CreateItemLocal(&itemData, &itemMeta, fmt.Sprintf("%v", FILM_NUMMER))
			if err != nil {
				fmt.Errorf("cannot create item #%v - %v -- %v", zoterogroup, FILM_NUMMER, err)
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

		if Verweis_auf_Publikation != "" {
			pubnote, err := grp.GetItemByOldidLocal(fmt.Sprintf("%v.pub", FILM_NUMMER))
			if err != nil {
				fmt.Errorf("cannot load item by oldid #%v - %v.tech -- %v", zoterogroup, FILM_NUMMER, err)
				break
			}
			//techItemData := zotero.ItemDataNote{}
			pubItemData := zotero.ItemGeneric{}
			pubItemData.ItemType = "note"
			pubItemData.Relations = make(map[string]zotero.ZoteroStringList)
			pubItemData.Tags = []zotero.ItemTag{}
			pubItemData.Collections = []string{}
			pubItemData.ParentItem = zotero.Parent(item.Key)
			pubItemData.Title = "Publikation"
			pubItemData.Note = strings.Replace(Verweis_auf_Publikation, "\n", "<br />\n", -1)

			pubnoteMeta := zotero.ItemMeta{}
			if pubnote == nil {
				pubnote, err = grp.CreateItemLocal(&pubItemData, &pubnoteMeta, fmt.Sprintf("%v.pub", FILM_NUMMER))
				if err != nil {
					fmt.Errorf("cannot create item #%v - %v.pub -- %v", zoterogroup, FILM_NUMMER, err)
					break
				}
			} else {
				pubnote.Data = pubItemData
				pubnote.Data.Version = pubnote.Version
				//item.Meta = itemMeta
				pubnote.Status = zotero.SyncStatus_Modified
				pubnote.Data.Key = pubnote.Key
				pubnote.UpdateLocal()
			}

		}

		technote, err := grp.GetItemByOldidLocal(fmt.Sprintf("%v.tech", FILM_NUMMER))
		if err != nil {
			fmt.Errorf("cannot load item by oldid #%v - %v.tech -- %v", zoterogroup, FILM_NUMMER, err)
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
		technoteMeta := zotero.ItemMeta{}

		if technote == nil {
			technote, err = grp.CreateItemLocal(&techItemData, &technoteMeta, fmt.Sprintf("%v.tech", FILM_NUMMER))
			if err != nil {
				fmt.Errorf("cannot create item #%v - %v.tech -- %v", zoterogroup, FILM_NUMMER, err)
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
				fmt.Errorf("cannot scan result data: %v", err)
				break
			}
			oldid := fmt.Sprintf("%v-%v", FILM_NUMMER, masterid)
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

			/*
				itemData.Note = fmt.Sprintf("Resolution: %vx%v", width, height)
				// wird schon klappen....
				if duration > 0 {
					d, _ := time.ParseDuration(fmt.Sprintf("%vs", duration))
					d = d.Round(time.Minute)
					h := d / time.Hour
					d -= h * time.Hour
					m := d / time.Minute
					d -= m * time.Minute
					s := d / time.Second
					itemData.Note += fmt.Sprintf("<br />\nDuration: %d:%02d:%02d", h, m, s)
				}
			*/
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
