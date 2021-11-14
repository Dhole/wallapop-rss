package walla

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGenFeed(t *testing.T) {
	query := Query{
		Keywords:       []string{"psp"},
		Ignores:        []string{},
		LocationName:   "Barcelona",
		LocationRadius: 5,
		MinPrice:       100,
		MaxPrice:       200,
	}
	queries := Queries{
		path:    ".",
		queries: map[string]Query{},
	}
	cfg := FeedsConfig{
		CacheTimeout:     1 * time.Second,
		UpdateQueryDelay: 60 * time.Minute,
	}
	feeds := NewFeeds(&queries, cfg)
	feed, err := feeds.genFeed(&query)
	require.Nil(t, err)

	// fmt.Printf("%#v\n", *feed)
	fmt.Printf("%+v\n", *feed.Items[0])
}
