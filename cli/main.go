package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/Dhole/wallapop-rss/walla"
)

func main() {
	keyword := flag.String("keyword", "", "search keyword")
	locationName := flag.String("locationName", "", "location place name")
	locationRadius := flag.Uint64("locationRadius", 5, "location radius")
	minPrice := flag.Uint64("minPrice", 0, "minimum price")
	maxPrice := flag.Uint64("maxPrice", 9999, "maximum price")
	flag.Parse()

	location, err := walla.GetLocation(*locationName)
	if err != nil {
		log.Fatal(err)
	}
	req := walla.ReqSearch{
		Distance:      float32(*locationRadius * 1000),
		Keywords:      *keyword,
		FiltersSource: "quick_filters",
		OrderBy:       "newest",
		MinSalePrice:  int(*minPrice),
		MaxSalePrice:  int(*maxPrice),
		Latitude:      location.Latitude,
		Longitude:     location.Longitude,
		Language:      "es_ES",
	}
	res, err := walla.Search(walla.SearchOpts{Age: 30 * 24 * time.Hour}, &req)
	if err != nil {
		log.Fatal(err)
	}
	resJSON, _ := json.MarshalIndent(res, "", "  ")
	fmt.Printf("%s\n", resJSON)

	// for i, object := range res.SearchObjects {
	// 	fmt.Printf("=== (%d) %s ===\n", i, object.ID)
	// 	item, err := walla.GetItem(object.ID)
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}
	// 	itemJSON, _ := json.MarshalIndent(item, "", "  ")
	// 	fmt.Printf("%s\n", itemJSON)
	// }
}
