package main

import (
	"database/sql"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/tominescu/pt-rss/config"
	"github.com/tominescu/pt-rss/rss"
)

const LINK_RETRY_TIMES = 3
const CREATE_TABLE_STMT = `CREATE TABLE IF NOT EXISTS urls (
	id INTEGER PRIMARY KEY AUTOINCREMENT NOT NULL,
	link VARCHAR(1024) NOT NULL,
	download_count INTEGER NOT NULL DEFAULT '0',
	update_time TimeStamp NOT NULL DEFAULT (datetime('now','localtime'))
	);`

var VERSION = "v1.2"

func main() {
	c := loadConfig()

	timeout := 30
	if 0 < c.Timeout && c.Timeout < 3600 {
		timeout = c.Timeout
	}

	httpClient := http.Client{
		Timeout: time.Duration(timeout) * time.Second,
	}

	if err := os.MkdirAll(c.SettingsDir, 0755); err != nil {
		log.Printf("Error create settings directory: %s", err)
		os.Exit(1)
	}

	dbPath := path.Join(c.SettingsDir, ".downloaded.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Open db file failed: %s\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// create table
	_, err = db.Exec(CREATE_TABLE_STMT)
	if err != nil {
		panic(err)
	}

	finish := make(chan int, 1)
	for _, site := range c.Sites {
		go handleSite(site, &httpClient, db, finish)
	}

	// Wait for finish
	for _ = range c.Sites {
		<-finish
	}
	log.Println("Exit because all goroutines are down")
}

func handleSite(site config.Site, httpClient *http.Client, db *sql.DB, finish chan<- int) {
	logPrefix := fmt.Sprintf("[%s]\t", site.Name)
	log := log.New(os.Stdout, logPrefix, log.LstdFlags)
	log.Printf("Start with url %s", site.Rss)

	// create download dir
	if err := os.MkdirAll(site.DownloadDir, 0755); err != nil {
		log.Printf("Error create download directory: %s", err)
		finish <- 1
	}

	firstTurn := true
	for {
		if firstTurn {
			firstTurn = false
		} else {
			time.Sleep(time.Duration(site.Interval) * time.Second)
		}

		rsp, err := httpClient.Get(site.Rss)
		if err != nil {
			log.Printf("Error fetching rss:%s", err)
			continue
		}
		data, err := ioutil.ReadAll(rsp.Body)
		rsp.Body.Close()
		if err != nil {
			log.Printf("Error read response", err)
			continue
		}
		r, err := rss.NewRss(data)
		if err != nil {
			log.Printf("Error parse rss")
			continue
		}
		links := r.GetLinks()
		var newLinks []string

		for _, link := range links {
			count, _ := getLinkCount(db, link)
			if count < LINK_RETRY_TIMES {
				newLinks = append(newLinks, link)
			}
		}
		log.Printf("Get %d links include %d new links", len(links), len(newLinks))
		for _, link := range newLinks {
			if err = handleLink(&site, link, log, httpClient); err != nil {
				incrLinkCount(db, link)
			} else {
				setLinkCount(db, link, LINK_RETRY_TIMES)
			}
		}
	}
}

func getLinkCount(db *sql.DB, link string) (int, error) {
	count := 0
	err := db.QueryRow("SELECT download_count FROM urls WHERE link=?", link).Scan(&count)
	switch {
	case err == sql.ErrNoRows:
		return count, nil
	case err != nil:
		panic(err)
	default:
		return count, err
	}
	return count, nil
}

func setLinkCount(db *sql.DB, link string, count int) error {
	_, err := db.Exec("DELETE FROM urls WHERE link=?", link)
	if err != nil {
		panic(err)
	}
	_, err = db.Exec("INSERT INTO urls(link, download_count) VALUES(?,?)", link, count)
	if err != nil {
		panic(err)
	}
	return err
}

func incrLinkCount(db *sql.DB, link string) {
	count, _ := getLinkCount(db, link)
	count++
	setLinkCount(db, link, count)
}

func handleLink(site *config.Site, link string, log *log.Logger, httpClient *http.Client) error {
	log.Printf("Handled link %s", link)
	rsp, err := httpClient.Get(link)
	if err != nil {
		log.Printf("Error download link %s : %s", link, err)
		return err
	}
	defer rsp.Body.Close()
	data, err := ioutil.ReadAll(rsp.Body)
	if err != nil {
		log.Printf("Error read link response %s : %s", link, err)
		return err
	}

	tmpVal := rsp.Header.Get("Content-Disposition")
	value, err := url.QueryUnescape(tmpVal)
	if err != nil {
		value = tmpVal
	}
	filename := getFileName(value)

	log.Printf("Get file %s", filename)
	filePath := path.Join(site.DownloadDir, filename)

	err = ioutil.WriteFile(filePath, data, 0644)
	if err != nil {
		log.Printf("Save File %s failed: %s", filename, err)
		return err
	}
	log.Printf("Torrent save to %s", filePath)

	return nil
}

func getFileName(value string) string {
	for _, str := range strings.Split(value, ";") {
		str = strings.TrimSpace(str)
		if strings.HasPrefix(str, "filename=") {
			return strings.TrimPrefix(str, "filename=")
		}
	}
	return ""
}

func loadConfig() *config.Config {
	configPath := flag.String("c", "config.json", "The config file path")
	help := flag.Bool("h", false, "Print this usage")
	version := flag.Bool("v", false, "Print version")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [-c config.json] [-h]\n\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.Parse()
	if *help {
		flag.Usage()
		os.Exit(0)
	}
	if *version {
		fmt.Fprintf(os.Stderr, "Version: %s\n\n", VERSION)
		os.Exit(0)
	}

	configData, err := ioutil.ReadFile(*configPath)
	if err != nil {
		loadConfigErr(err)
	}

	c, err := config.NewConfig(configData)
	if err != nil {
		loadConfigErr(err)
	}
	return c
}

func loadConfigErr(err error) {
	fmt.Fprintf(os.Stderr, "Error Loading config: %s\n\n", err)
	flag.Usage()
	os.Exit(1)
}
