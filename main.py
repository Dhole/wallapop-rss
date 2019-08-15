#!/usr/bin/env python3

import re
import sys
import requests
import toml
import time
from flask import Flask
from datetime import datetime
from PyRSS2Gen import RSS2, RSSItem, Guid
from threading import RLock

from signature import get_timestamp, get_signature

USER_AGENT = 'Mozilla/5.0 (X11; Linux x86_64; rv:67.0) Gecko/20100101 Firefox/67.0'
URL = 'https://es.wallapop.com'
URLAPIV3 = 'https://api.wallapop.com/api/v3'

class Cache:
    def __init__(self, updater, expiration):
        self.updater = updater
        self.expiration = expiration
        self.dict = {}
        self.lock = RLock()

    def clean(self):
        self.lock.acquire()
        max_timestamp = int(time.time()) - self.expiration
        try:
            for key in [k for k, v in self.dict.items() if v[1] < max_timestamp]:
                # print('# Cleaning', key, 'from cache')
                del self.dict[key]
        finally:
            self.lock.release()

    def get(self, key):
        self.clean()
        self.lock.acquire()
        value = None
        try:
            if key not in self.dict:
                _value = self.updater(key)
                timestamp = int(time.time())
                # print('# Adding', key, 'to cache')
                self.dict[key] = (_value, timestamp)
            value = self.dict[key]
        finally:
            self.lock.release()
        return value[0]

def get(url, params, cookies = {}):
    timestamp = get_timestamp()
    signature = get_signature(url, 'get', timestamp)
    headers = {'User-Agent': USER_AGENT, 'Timestamp': timestamp, 'X-Signature': signature}
    r = requests.get(url, params=params, headers=headers, cookies=cookies)
    return r.json()

def get_location(place_id):
    return get(f'{URL}/maps/here/place', {'placeId': place_id})

def search(keywords, location, location_radius, min_price, max_price):
    return get(f'{URLAPIV3}/general/search',
            {
                'distance': str(location_radius*1000),
                'keywords': keywords,
                'filters_source': 'quick_filters',
                'order_by': 'newest',
                'min_sale_price': min_price,
                'max_sale_price': max_price,
                'latitude': str(location['latitude']),
                'longitude': str(location['longitude']),
                'language': 'es_ES'
            })

def get_date(item_id):
    item = get(f'{URLAPIV3}/items/{item_id}', {})
    # print(item)
    # print('>>>', item_id)
    date = item['content']['modified_date']
    return datetime.utcfromtimestamp(date//1000)
    # url = f"{URL}/item/{item['web_slug']}"
    # res = requests.get(url).text
    # match = re.search('(<div class=\"card-product-detail-user-stats-published\">)([^<]*)(</div>)',
    #         res)
    # date_str = match.group(2)
    # print(date_str)

app = Flask(__name__)
queries = {}
item_date_cache = Cache(get_date, 12 * 3600)

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

        for item in result['search_objects']:
            item_id = item['id']
            if item_id in item_ids:
                continue

            # date = get_date(item_id)
            date = item_date_cache.get(item_id)
            # print(f"{date.strftime('%Y-%m-%d %H:%M:%S')} - {item['price']} - {item['title']}")
            description = item['description'] + '<br/>'
            for image in item['images']:
                description += f'<img src="{image["medium"]}"><br/>'
            rss_items.append(RSSItem(
                    title=f"{item['title']} - {item['price']} {item['currency']}",
                    link=f"https://es.wallapop.com/item/{item['web_slug']}",
                    description=description,
                    author=item['user']['micro_name'],
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
    return feed.to_xml('utf-8'), 200, {'Content-Type': 'text/xml; charset=utf-8'}

if __name__ == "__main__":
    port = 8080
    if len(sys.argv) > 1:
        port = int(sys.argv[1])
    with open('queries.toml') as file:
        data = file.read()
        queries = toml.loads(data)
    app.run(host="0.0.0.0", port=port, threaded=True)
