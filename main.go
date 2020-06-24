package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/google/go-querystring/query"
	log "github.com/sirupsen/logrus"
)

const (
	USER_AGENT           = "Mozilla/5.0 (X11; Linux x86_64; rv:67.0) Gecko/20100101 Firefox/67.0"
	URL                  = "https://es.wallapop.com"
	URLAPIV3             = "https://api.wallapop.com/api/v3"
	DEFAULT_QUERIES_PATH = "./queries.toml"
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
		return entry.Value, nil
	}
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
		return fmt.Errorf("doing http request: %w", err)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading http response body: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.WithField("url", resp.Request.URL).WithField("body", string(body)).Error("bad http request")
		return fmt.Errorf("http status code is %v", resp.StatusCode)
	}
	log.Debug(resp.Request.URL)
	fmt.Println("###")
	fmt.Print(string(body))
	fmt.Println("###")
	if err := json.Unmarshal(body, res); err != nil {
		return fmt.Errorf("json unmarshaling http response body: %w", err)
	}
	return nil
}

type ReqMapsHerePlace struct {
	PlaceId string `url:"placeId"`
}

type ResMapsHerePlace struct {
	Latitude  float32 `json:"latitude"`
	Longitude float32 `json:"longitude"`
}

type ReqSearch struct {
	Distance      string `url:"distance"`
	Keywords      string `url:"keywords"`
	FiltersSource string `url:"filters_source"`
	OrderBy       string `url:"order_by"`
	MinSalePrice  int    `url:"min_sale_price"`
	MaxSalePrice  int    `url:"max_sale_price"`
	Latitude      string `url:"latitude"`
	Longitude     string `url:"longitude"`
	Language      string `url:"language"`
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
	Distance    int     `json:"distance"`
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
	} `json:"urls_by_size"`
}

type ResItem struct {
	ID      string `json:"id"`
	Content struct {
		ModifiedDate int64       `json:"modified_date"`
		Images       []ItemImage `json:"images"`
	} `json:"content"`
}

func main() {
	log.SetLevel(log.DebugLevel)
	q, err := NewQueries(DEFAULT_QUERIES_PATH)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%+v\n", q.Get())

	// var res ResMapsHerePlace
	// if err := get(fmt.Sprintf("%v/maps/here/place", URL), ReqMapsHerePlace{"Barcelona"}, &res); err != nil {
	// 	panic(err)
	// }
	// fmt.Printf("%+v\n", res)

	// var res ResSearch
	// if err := get(fmt.Sprintf("%v/general/search", URLAPIV3),
	// 	ReqSearch{
	// 		Distance:      "5000",
	// 		Keywords:      "kindle",
	// 		FiltersSource: "quick_filters",
	// 		OrderBy:       "newest",
	// 		MinSalePrice:  0,
	// 		MaxSalePrice:  999,
	// 		Latitude:      "41.38804",
	// 		Longitude:     "2.17001",
	// 		Language:      "es_ES",
	// 	},
	// 	&res); err != nil {
	// 	panic(err)
	// }
	// fmt.Printf("%+v\n", res.SearchObjects[0])

	itemID := "v6g45xw4r56e"
	var res ResItem
	if err := get(fmt.Sprintf("%v/items/%v", URLAPIV3, itemID),
		struct{}{}, &res); err != nil {
		panic(err)
	}
	fmt.Printf("%+v\n", res)
}
