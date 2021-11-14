package walla

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
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

func GetParamsString(url string, params string, res interface{}) (*http.Response, error) {
	signature, timestamp := signNow(url, "get")

	req, err := http.NewRequest("GET", fmt.Sprintf("%s?%s", url, params), nil)
	if err != nil {
		return nil, fmt.Errorf("building http request: %w", err)
	}
	req.Header.Set("User-Agent", USER_AGENT)
	req.Header.Set("Timestamp", timestamp)
	req.Header.Set("X-Signature", signature)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.WithField("url", url).Error("Failed http request")
		return nil, fmt.Errorf("doing http request: %w", err)
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading http response body: %w", err)
	}
	log.WithField("url", url).Debug("HTTP GET")
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.WithField("url", url).WithField("body", string(body)).WithField("params", params).
			Error("Bad http request")
		return nil, fmt.Errorf("http status code is %v", resp.StatusCode)
	}
	// fmt.Printf("DBG Req: %+v\n", req)
	// log.Debug(resp.Request.URL)
	// fmt.Println("###")
	// fmt.Print(string(body))
	// fmt.Println("\n###")
	if err := json.Unmarshal(body, res); err != nil {
		log.WithField("url", url).WithField("body", string(body)).Error("Bad json body")
		return nil, fmt.Errorf("json unmarshaling http response body: %w", err)
	}
	return resp, nil
}

func Get(url string, params interface{}, res interface{}) (*http.Response, error) {
	v, err := query.Values(params)
	if err != nil {
		return nil, fmt.Errorf("parsing url params: %w", err)
	}
	return GetParamsString(url, v.Encode(), res)
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
	// Step           int     `url:"step"`
	// SearchID       string  `url:"search_id"`
	// PaginationDate string  `url:"pagination_date"`
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

type NextPage struct {
	Raw            string
	Step           int
	SearchID       string
	PaginationDate time.Time
}

func NewNextPage(raw string) (*NextPage, error) {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return nil, err
	}
	paginationDate, err := time.Parse(time.RFC3339, values.Get("pagination_date"))
	if err != nil {
		return nil, fmt.Errorf("can't parse pagination_date from NextPage")
	}
	step, err := strconv.Atoi(values.Get("step"))
	if err != nil {
		return nil, fmt.Errorf("can't parse step from NextPage")
	}
	return &NextPage{
		Raw:            raw,
		Step:           step,
		SearchID:       values.Get("search_id"),
		PaginationDate: paginationDate,
	}, nil
}

type ResSearch struct {
	SearchObjects []SearchObject `json:"search_objects"`
	// NextPage      NextPage
}

type ItemImage struct {
	URLs struct {
		Big string `json:"big"`
	} `json:"urls"`
}

type ResItem struct {
	ID           string      `json:"id"`
	ModifiedDate int64       `json:"modified_date"`
	Images       []ItemImage `json:"images"`
}

func GetLocation(place string) (*ResMapsHerePlace, error) {
	var res ResMapsHerePlace
	if _, err := Get(fmt.Sprintf("%v/maps/here/place", URL), ReqMapsHerePlace{place}, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

type SearchOpts struct {
	Age time.Duration
}

func Search(opts SearchOpts, req *ReqSearch) (*ResSearch, error) {
	var res ResSearch
	// req := *_req
	// req.Step = 1
	limit := time.Now().Add(-opts.Age)
	v, err := query.Values(req)
	if err != nil {
		return nil, fmt.Errorf("parsing url params: %w", err)
	}
	params := v.Encode()
	for {
		var tmpRes ResSearch
		resp, err := GetParamsString(fmt.Sprintf("%v/general/search", URLAPIV3),
			params,
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
			&tmpRes)
		if err != nil {
			return nil, err
		}
		res.SearchObjects = append(res.SearchObjects, tmpRes.SearchObjects...)
		nextPage, err := NewNextPage(resp.Header.Get("X-NextPage"))
		if err != nil {
			return nil, err
		}
		if limit.After(nextPage.PaginationDate) {
			break
		}
		params = nextPage.Raw
		// req.PaginationDate = nextPage.PaginationDate.Format(time.RFC3339)
		// req.Step = nextPage.Step
		// req.SearchID = nextPage.SearchID
	}
	return &res, nil
}

func GetItem(itemID string) (*ResItem, error) {
	var res ResItem
	if _, err := Get(fmt.Sprintf("%v/items/%v", URLAPIV3, itemID),
		struct{}{}, &res); err != nil {
		return nil, err
	}
	// fmt.Printf("DBG %+v\n", res)
	return &res, nil
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
			func(key string) (interface{}, error) { return GetItem(key) },
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
		Updated:     now,
		Items:       make([]*feeds.Item, 0),
	}
	location, err := GetLocation(query.LocationName)
	if err != nil {
		return nil, err
	}
	itemIDs := make(map[string]bool)
	for _, keyword := range query.Keywords {
		result, err := Search(
			SearchOpts{Age: 15 * 24 * time.Hour},
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
			for _, image := range itemData.Images {
				src := fmt.Sprintf("%v1024", strings.TrimSuffix(image.URLs.Big, "800"))
				description += fmt.Sprintf(`<img src="%v"><br/>`, src)
			}
			date := time.Unix(itemData.ModifiedDate, 0)
			feed.Items = append(feed.Items, &feeds.Item{
				Id:          item.ID,
				Title:       fmt.Sprintf("%v - %v %v", item.Title, item.Price, item.Currency),
				Link:        &feeds.Link{Href: fmt.Sprintf("%v/item/%v", URL, item.WebSlug)},
				Description: description,
				Author:      &feeds.Author{Name: item.User.MicroName},
				Created:     date,
				Updated:     date,
			})
		}
	}
	return &feed, nil
}
