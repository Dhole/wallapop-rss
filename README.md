# wallapop-rss

Wallapop RSS query feed

# Requirements

- `go`

# Usage

The default listening port is 8080, but it can be changed with the `-address` flag.
The configuration file by default is `./queries.toml` but can be changed with
the `-queries` flag.  If the queries files is updated, the process
automatically loads the new queries.

```
./wallapop-rss
Usage of ./wallapop-rss:
  -addr string
        http listening address (default "127.0.0.1:8080")
  -cacheTimeout int
        timeout for the item cache (hours) (default 12)
  -debug
        enable debug logs
  -queries string
        queries file path (default "./queries.toml")
  -updateDelay int
        delay between concurrent query updates (seconds) (default 1)
  -updateInterval int
        interval between query updates (minutes) (default 15)
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

# Docker

Build docker image
```sh
docker build -t wallapop-rss . -f Dockerfile
```

Create a local user for the container

```sh
addgroup -S wallapop-rss
adduser -S -D -H -G wallapop-rss wallapop-rss
```

Run the container with the created user.  The example `docker-compose.yml`
provided expects the configuration to be in `data/queries.toml`.  Modify it as
you see fit.
```sh
name="wallapop-rss"
export SVC_USER="$(id -u ${name})"
export SVC_GROUP="$(id -g ${name})"
docker-compose --file docker-compose.yml up -d
```

# License

The code is released under the 3-clause BSD License.

# History

The original version of wallapop-rss was written in python.  This is the python specific documentation:

## Requirements

- `python-flask`
- `python-pyrss2gen`

## Usage

The default listening port is 8080, but it can be changed with the first argument.
The configuration file must be named `queries.toml` and be in the path where the main program is executed.

```
./main.py [PORT]
```

The generated endpoints will be of the form `/rss/FEED_NAME`.
