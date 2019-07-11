# Crawlr

A simple/toy concurrent web crawler written in Go.

## Installation

```
go get github.com/jamesmccann/crawlr
```

## Usage

Usage from command line:

```
Usage: crawlr [options] <url>
  -c int
    	Number of concurrent workers for crawling. (default 1)
  -d int
    	Search depth. Set to -1 to crawl all pages reachable from the initial page. (default 1)
  -exclude string
    	Comma-separated list of regexp for urls to exclude from crawling.
  -f string
    	Output sitemap format (xml|simple). (default "xml")
  -h	Prints this help message.
  -v	Enables verbose debug logging.
```

### Example

```
$ crawlr -c 1 -d -1 -f simple http://jamesmccann.nz
Crawl results for http://jamesmccann.nz
  - http://jamesmccann.nz/2014/11/27/bundling-npm-modules-through-webpack-and-rails-asset-pipeline.html
    - http://jamesmccann.nz
  - http://jamesmccann.nz/2017/03/03/rebuilding-powerswitch.html
    - http://jamesmccann.nz/images/2017/03/03/powerswitch-rebuild-header.png
    - http://jamesmccann.nz/images/2017/03/03/powerswitch-results-trends.png
    - http://jamesmccann.nz/images/2017/03/03/powerswitch-revision-sets.png
  - http://jamesmccann.nz/2014/09/18/optimising-expensive-aggregation-in-activerecord-with-view-backed-models.html
  - http://jamesmccann.nz/2015/04/18/building-a-tessel-compatible-driver-for-the-mpu-6050-accelerometer-and-gyroscope.html
```

### Output Formats

There are two supported output formats, `xml` and `simple`. `xml` will
output an [XML sitemap format](http://sitemap.org). Simple will output a
simple list of all crawled pages and nested links found on those pages.

## Future Improvements

- [ ] "Events" system to allow for user-side hooks - e.g. on each page,
  on each link, on each HTTP request.
- [ ] Real-time progress display with statistics for number of currently
  queued fetches, number of in-flight requests, etc.
- [ ] Rate-limiting, automated detection avoidance.
- [ ] JSON Formatter showing "tree" relationship between pages.
