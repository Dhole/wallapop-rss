#!/usr/bin/env python3

import sys
import requests
import toml
from flask import Flask
from datetime import datetime
from PyRSS2Gen import RSS2, RSSItem, Guid

USER_AGENT = 'Mozilla/5.0 (X11; Linux x86_64; rv:67.0) Gecko/20100101 Firefox/67.0'
URL = 'https://es.wallapop.com'

app = Flask(__name__)
queries = {}

def get(url, params, cookies = {}):
    headers = {'User-Agent': USER_AGENT}
    r = requests.get(url, params=params, headers=headers, cookies=cookies)
    return r.json()

def get_location(place_id):
    return get(f'{URL}/maps/here/place', {'placeId': place_id})

def search(keywords, location, location_radius, min_price, max_price):
    return get(f'{URL}/rest/items',
            {'dist': str(location_radius), 'kws': keywords, 'filters_source': 'quick_filters', 'order': 'creationDate-des',
                'minPrice': min_price, 'maxPrice': max_price},
            {'searchLat': str(location['latitude']), 'searchLng': str(location['longitude'])})

# Heavily inspired by https://github.com/tanrax/wallaviso/blob/master/app.py
@app.route('/rss/<string:id>')
def rss_view(id):
    query = queries[id]
    keywords = query['keywords']
    location_name = query['location_name']
    location_radius = query['location_radius']
    min_price = query['min_price']
    max_price = query['max_price']

    location = get_location(location_name)
    rss_items = []

    item_ids = set()
    for keyword in keywords:
        result = search(keywords, location, location_radius, min_price, max_price)

        for item in result['items']:
            item_id = item['itemId']
            if item_id in item_ids:
                continue

            date = datetime.utcfromtimestamp(item['publishDate']//1000)
            # print(f"{date.strftime('%Y-%m-%d %H:%M:%S')} - {item['price']} - {item['title']}")
            rss_items.append(RSSItem(
                    title=f"{item['title']} - {item['salePrice']}{item['currency']['symbol']}",
                    link=f"https://es.wallapop.com/item/{item['url']}",
                    description=item['description'],
                    author=item['sellerUser']['microName'],
                    guid=Guid(str(item_id)),
                    pubDate=date
                    )
                )
            item_ids.add(item_id)

    lastBuildDate = datetime.now()
    if len(rss_items) != 0:
        lastBuildDate = rss_items[0].pubDate

    feed = RSS2(
        title=f"{keywords} - Wallapop RSS",
        link="http://es.wallapop.com",
        description="Wallapop RSS feed.",
        language="es-ES",
        lastBuildDate=lastBuildDate,
        items=rss_items
    )
    return feed.to_xml(), 200, {'Content-Type': 'text/xml; charset=utf-8'}

if __name__ == "__main__":
    port = 8080
    if len(sys.argv) > 1:
        port = int(sys.argv[1])
    with open('queries.toml') as file:
        data = file.read()
        queries = toml.loads(data)
    app.run(host="0.0.0.0", port=port)
