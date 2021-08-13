package main

import (
	"flag"
	"os"
	"time"

	"github.com/Dhole/wallapop-rss/walla"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

type FileWatch struct {
	Changed bool
	Error   error
}

// watchFile spawns a goroutine that watches the file in filePath and notifies
// about changes via the returned channel.
func watchFile(filePath string) (chan FileWatch, error) {
	saveStat, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}
	notifications := make(chan FileWatch)
	go func() {
		for {
			stat, err := os.Stat(filePath)
			if err != nil {
				notifications <- FileWatch{Changed: false, Error: err}
				continue
			}

			if stat.Size() != saveStat.Size() || stat.ModTime() != saveStat.ModTime() {
				saveStat = stat
				notifications <- FileWatch{Changed: true, Error: nil}
				continue
			}

			time.Sleep(4 * time.Second)
		}
	}()
	return notifications, nil
}

func main() {
	addr := flag.String("addr", "127.0.0.1:8080", "http listening address")
	debug := flag.Bool("debug", false, "enable debug logs")
	queriesPath := flag.String("queries", "./queries.toml", "queries file path")
	cacheTimeoutHours := flag.Int64("cacheTimeout", 12, "timeout for the item cache (hours)")
	updateQueryDelaySeconds := flag.Int64("updateDelay", 1, "delay between concurrent query updates (seconds)")
	updateIntervalMinutes := flag.Int64("updateInterval", 15, "interval between query updates (minutes)")
	flag.Parse()

	cacheTimeout := time.Duration(*cacheTimeoutHours) * time.Hour
	updateQueryDelay := time.Duration(*updateQueryDelaySeconds) * time.Second
	updateInterval := time.Duration(*updateIntervalMinutes) * time.Minute

	if *debug {
		log.SetLevel(log.DebugLevel)
	}

	log.Info("Loading queries file for the first time...")
	queries, err := walla.NewQueries(*queriesPath)
	if err != nil {
		panic(err)
	}
	queriesUpdate, err := watchFile(*queriesPath)
	if err != nil {
		panic(err)
	}

	go func() {
		for {
			update := <-queriesUpdate
			if update.Error != nil {
				log.WithField("file", queriesPath).WithError(update.Error).
					Error("Failed watching queries file")
				continue
			}
			if err := queries.Load(); err != nil {
				log.WithField("file", queriesPath).WithError(err).
					Error("Failed parsing queries file")
				continue
			}
			log.WithField("file", queriesPath).
				Info("updated queries feeds")
		}
	}()

	myFeeds := walla.NewFeeds(queries, walla.FeedsConfig{
		CacheTimeout:     cacheTimeout,
		UpdateQueryDelay: updateQueryDelay,
	})
	log.Info("Updating queries feeds for the first time...")
	myFeeds.Update()

	go func() {
		for {
			time.Sleep(updateInterval)
			myFeeds.Update()
		}
	}()

	r := gin.Default()
	r.GET("/rss/:name", func(c *gin.Context) {
		name := c.Param("name")
		feed, err := myFeeds.Get(name)
		if err != nil {
			log.WithError(err).WithField("name", name).Error("Unable to get feed")
			c.JSON(404, gin.H{
				"error": err,
			})
			return
		}
		rss, err := feed.ToRss()
		if err != nil {
			log.WithError(err).WithField("name", name).Error("Unable build rss feed")
			c.JSON(404, gin.H{
				"error": err,
			})
			return
		}
		c.Data(200, "application/xml", []byte(rss))
	})
	log.WithField("addr", *addr).Info("Serving http")
	r.Run(*addr)
}
