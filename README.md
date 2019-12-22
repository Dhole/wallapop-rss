# wallapop-rss

Wallapop RSS query feed

# Requirements

- `python-flask`
- `python-pyrss2gen`

# Usage

The default listening port is 8080, but it can be changed with the first argument.
The configuration file must be named `queries.toml` and be in the path where the main program is executed.

```
./main.py [PORT]
```

The generated endpoints will be of the form `/rss/FEED_NAME`.

# Example config

```
[iphone] # RSS feed name
keywords = ["iphone 7", "iphone 6S"] # List of keywords to search for
ignores = ["ipad"] # Ignore results both by title and description content
location_name = "Barcelona" # City location
location_radius = 5 # Radius in Km from the location
min_price = 0 # Minimum price in EUR
max_price = 200 # Maximum price in EUR
```
# License

The code is released under the 3-clause BSD License.
