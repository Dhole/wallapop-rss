package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/gin-gonic/gin"
	"github.com/google/go-querystring/query"
	"github.com/gorilla/feeds"
	log "github.com/sirupsen/logrus"
)

const (
	USER_AGENT = "Mozilla/5.0 (X11; Linux x86_64; rv:67.0) Gecko/20100101 Firefox/67.0"
	URL        = "https://es.wallapop.com"
	URLAPIV3   = "https://api.wallapop.com/api/v3"
)

type Query struct {
	Keywords       []string `toml:"keywords"`
	Ignores        []string `toml:"ignores"`
	LocationName   string   `toml:"location_name"`
	LocationRadius int      `toml:"location_radius"`
	MinPrice       int      `toml:"min_price"`
	MaxPrice       int      `toml:"max_price"`
}

type Queries struct {
	path    string
	queries map[string]Query
	m       sync.RWMutex
}

func (q *Queries) Get() map[string]Query {
	q.m.RLock()
	defer q.m.RUnlock()
	return q.queries
}

func (q *Queries) set(queries map[string]Query) {
	q.m.Lock()
	defer q.m.Unlock()
	q.queries = queries
}

func (q *Queries) Load() error {
	queries := make(map[string]Query)
	if _, err := toml.DecodeFile(q.path, &queries); err != nil {
		return err
	}
	for name, _ := range queries {
		for i, ignore := range queries[name].Ignores {
			queries[name].Ignores[i] = strings.ToLower(ignore)
		}
	}
	q.set(queries)
	return nil
}

func NewQueries(path string) (*Queries, error) {
	q := Queries{path: path}
	if err := q.Load(); err != nil {
		return nil, err
	}
	return &q, nil
}

type CacheEntry struct {
	Timestamp time.Time
	Value     interface{}
}

type Cache struct {
	expiration time.Duration
	entries    map[string]CacheEntry
	fetchFn    func(key string) (interface{}, error)
	m          sync.RWMutex
}

func NewCache(fetchFn func(key string) (interface{}, error), expiration time.Duration) *Cache {
	return &Cache{
		expiration: expiration,
		entries:    make(map[string]CacheEntry),
		fetchFn:    fetchFn,
	}
}

func (c *Cache) Get(key string) (interface{}, error) {
	c.Clean()
	c.m.RLock()
	entry, ok := c.entries[key]
	c.m.RUnlock()
	if ok {
		log.WithField("key", key).Debug("Cache hit")
		return entry.Value, nil
	}
	log.WithField("key", key).Debug("Cache miss")
	value, err := c.fetchFn(key)
	if err != nil {
		return nil, err
	}
	c.m.Lock()
	c.entries[key] = CacheEntry{
		Timestamp: time.Now(),
		Value:     value,
	}
	c.m.Unlock()
	return value, nil
}

func (c *Cache) Clean() {
	c.m.Lock()
	defer c.m.Unlock()
	maxTimestamp := time.Now().Add(-c.expiration)
	for key, entry := range c.entries {
		if entry.Timestamp.Before(maxTimestamp) {
			delete(c.entries, key)
		}
	}
}

var KEY = []byte("Tm93IHRoYXQgeW91J3ZlIGZvdW5kIHRoaXMsIGFyZSB5b3UgcmVhZHkgdG8gam9pbiB1cz8gam9ic0B3YWxsYXBvcC5jb20==")

func sign(url, method, timestamp string) string {
	req := strings.TrimPrefix(url, "https://api.wallapop.com")
	msg := fmt.Sprintf("%s|%s|%s|", strings.ToUpper(method), req, timestamp)
	h := hmac.New(sha256.New, KEY)
	h.Write([]byte(msg))
	signature := h.Sum(nil)
	return base64.StdEncoding.EncodeToString(signature)
}

func signNow(url, method string) (string, string) {
	timestamp := fmt.Sprintf("%v", time.Now().Unix())
	return sign(url, method, timestamp), timestamp
}

func get(url string, params interface{}, res interface{}) error {
	signature, timestamp := signNow(url, "get")

	v, err := query.Values(params)
	if err != nil {
		return fmt.Errorf("parsing url params: %w", err)
	}
	req, err := http.NewRequest("GET", fmt.Sprintf("%s?%s", url, v.Encode()), nil)
	if err != nil {
		return fmt.Errorf("building http request: %w", err)
	}
	req.Header.Set("User-Agent", USER_AGENT)
	req.Header.Set("Timestamp", timestamp)
	req.Header.Set("X-Signature", signature)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.WithField("url", resp.Request.URL).Error("Failed http request")
		return fmt.Errorf("doing http request: %w", err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading http response body: %w", err)
	}
	log.WithField("url", resp.Request.URL).Debug("HTTP GET")
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.WithField("url", resp.Request.URL).WithField("body", string(body)).Error("Bad http request")
		return fmt.Errorf("http status code is %v", resp.StatusCode)
	}
	// log.Debug(resp.Request.URL)
	// fmt.Println("###")
	// fmt.Print(string(body))
	// fmt.Println("\n###")
	if err := json.Unmarshal(body, res); err != nil {
		log.WithField("url", resp.Request.URL).WithField("body", string(body)).Error("Bad json body")
		return fmt.Errorf("json unmarshaling http response body: %w", err)
	}
	return nil
}

type ReqMapsHerePlace struct {
	PlaceID string `url:"placeId"`
}

type ResMapsHerePlace struct {
	Latitude  float32 `json:"latitude"`
	Longitude float32 `json:"longitude"`
}

type ReqSearch struct {
	Distance      float32 `url:"distance"`
	Keywords      string  `url:"keywords"`
	FiltersSource string  `url:"filters_source"`
	OrderBy       string  `url:"order_by"`
	MinSalePrice  int     `url:"min_sale_price"`
	MaxSalePrice  int     `url:"max_sale_price"`
	Latitude      float32 `url:"latitude"`
	Longitude     float32 `url:"longitude"`
	Language      string  `url:"language"`
}

type User struct {
	ID        string `json:"id"`
	MicroName string `json:"micro_name"`
	Image     Image  `json:"images"`
}

type Image struct {
	Original string `json:"original"`
}

type Flags struct {
	Pending  bool `json:"pending"`
	Sold     bool `json:"sold"`
	Reserved bool `json:"reserved"`
	Banned   bool `json:"banned"`
	Expired  bool `json:"expired"`
	OnHold   bool `json:"onhold"`
}

type SearchObject struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Distance    float32 `json:"distance"`
	Images      []Image `json:"images"`
	User        User    `json:"user"`
	Flags       Flags   `json:"flags"`
	Price       float32 `json:"price"`
	Currency    string  `json:"currency"`
	WebSlug     string  `json:"web_slug"`
}

type ResSearch struct {
	SearchObjects []SearchObject `json:"search_objects"`
}

type ItemImage struct {
	ID         string `json:"id"`
	URLsBySize struct {
		Original string `json:"original"`
		Medium   string `json:"medium"`
	} `json:"urls_by_size"`
}

type ResItem struct {
	ID      string `json:"id"`
	Content struct {
		ModifiedDate int64       `json:"modified_date"`
		Images       []ItemImage `json:"images"`
	} `json:"content"`
}

func getLocation(place string) (*ResMapsHerePlace, error) {
	var res ResMapsHerePlace
	if err := get(fmt.Sprintf("%v/maps/here/place", URL), ReqMapsHerePlace{place}, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func search(req *ReqSearch) (*ResSearch, error) {
	var res ResSearch
	if err := get(fmt.Sprintf("%v/general/search", URLAPIV3),
		*req,
		// ReqSearch{
		// 	Distance:      5000,
		// 	Keywords:      "kindle",
		// 	FiltersSource: "quick_filters",
		// 	OrderBy:       "newest",
		// 	MinSalePrice:  0,
		// 	MaxSalePrice:  999,
		// 	Latitude:      41.38804,
		// 	Longitude:     2.17001,
		// 	Language:      "es_ES",
		// },
		&res); err != nil {
		return nil, err
	}
	return &res, nil
}

func getItem(itemID string) (*ResItem, error) {
	var res ResItem
	if err := get(fmt.Sprintf("%v/items/%v", URLAPIV3, itemID),
		struct{}{}, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

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

type FeedsConfig struct {
	CacheTimeout     time.Duration
	UpdateQueryDelay time.Duration
}

type Feeds struct {
	queries   *Queries
	itemCache *Cache
	feeds     map[string]*feeds.Feed
	cfg       FeedsConfig
	m         sync.RWMutex
}

func NewFeeds(queries *Queries, cfg FeedsConfig) *Feeds {
	return &Feeds{
		queries: queries,
		itemCache: NewCache(
			func(key string) (interface{}, error) { return getItem(key) },
			cfg.CacheTimeout),
		feeds: make(map[string]*feeds.Feed),
		cfg:   cfg,
	}
}

var (
	ErrFeedNotFound = errors.New("feed not found")
)

func (f *Feeds) Get(name string) (*feeds.Feed, error) {
	f.m.RLock()
	defer f.m.RUnlock()
	feed, ok := f.feeds[name]
	if !ok {
		return nil, ErrFeedNotFound
	}
	return feed, nil
}

func (f *Feeds) Update() {
	queries := f.queries.Get()
	type NameAndFeed struct {
		Name string
		Feed *feeds.Feed
	}
	ch := make(chan NameAndFeed)
	for name, query := range queries {
		go func(name string, query Query) {
			feed, err := f.genFeed(&query)
			if err != nil {
				log.WithError(err).WithField("name", name).Error("Unable to generate feed")
				ch <- NameAndFeed{Feed: nil, Name: name}
				return
			}
			ch <- NameAndFeed{Feed: feed, Name: name}
		}(name, query)
		time.Sleep(f.cfg.UpdateQueryDelay)
	}
	for i := 0; i < len(queries); i++ {
		select {
		case NameAndFeed := <-ch:
			if NameAndFeed.Feed == nil {
				continue
			}
			f.m.Lock()
			f.feeds[NameAndFeed.Name] = NameAndFeed.Feed
			f.m.Unlock()
		}

	}
}

func (f *Feeds) genFeed(query *Query) (*feeds.Feed, error) {
	now := time.Now()
	feed := feeds.Feed{
		Title:       fmt.Sprintf("%v - Wallapop RSS v2", query.Keywords),
		Link:        &feeds.Link{Href: "http://es.wallapop.com"},
		Description: "Wallapop RSS feed.",
		Author:      &feeds.Author{Name: "Dhole", Email: "dhole@riseup.net"},
		Created:     now,
		Items:       make([]*feeds.Item, 0),
	}
	location, err := getLocation(query.LocationName)
	if err != nil {
		return nil, err
	}
	itemIDs := make(map[string]bool)
	for _, keyword := range query.Keywords {
		result, err := search(
			&ReqSearch{
				Distance:      float32(query.LocationRadius * 1000),
				Keywords:      keyword,
				FiltersSource: "quick_filters",
				OrderBy:       "newest",
				MinSalePrice:  query.MinPrice,
				MaxSalePrice:  query.MaxPrice,
				Latitude:      location.Latitude,
				Longitude:     location.Longitude,
				Language:      "es_ES",
			},
		)
		if err != nil {
			return nil, err
		}
		items := result.SearchObjects
		for _, item := range items {
			if _, ok := itemIDs[item.ID]; ok {
				continue
			}
			ignoreItem := false
			for _, ignore := range query.Ignores {
				if strings.Contains(item.Description, ignore) {
					ignoreItem = true
					break
				}
			}
			if ignoreItem {
				continue
			}
			itemDataEntry, err := f.itemCache.Get(item.ID)
			if err != nil {
				return nil, err
			}
			itemData := itemDataEntry.(*ResItem)
			description := item.Description + "<br/>"
			for _, image := range itemData.Content.Images {
				description += fmt.Sprintf(`<img src="%v"><br/>`, image.URLsBySize.Medium)
			}
			feed.Items = append(feed.Items, &feeds.Item{
				Id:          item.ID,
				Title:       fmt.Sprintf("%v - %v %v", item.Title, item.Price, item.Currency),
				Link:        &feeds.Link{Href: fmt.Sprintf("%v/item/%v", URL, item.WebSlug)},
				Description: description,
				Author:      &feeds.Author{Name: item.User.MicroName},
				Created:     time.Unix(itemData.Content.ModifiedDate/1000, 0),
			})
		}
	}
	return &feed, nil
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
	queries, err := NewQueries(*queriesPath)
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

	myFeeds := NewFeeds(queries, FeedsConfig{
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
