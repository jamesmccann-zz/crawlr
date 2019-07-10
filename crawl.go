package crawlr

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	log "github.com/sirupsen/logrus"
)

// Default options for Crawler if not overidden.
var DefaultOpts = Opts{
	// Scrape all pages reachable on a domain.
	Depth: -1,

	// Ignore external pages on different domains.
	FollowExt: false,

	// Use 10 workers by default.
	NumWorkers: 10,

	// No default exclusion list.
	Exclude: []string{},

	// Default to 5s timeout per individual page fetch.
	FetchTimeout: 5 * time.Second,
}

// Page represents a crawl result including url, title, and timestamp for the fetch.
type Page struct {
	URL          url.URL
	Title        string
	FetchedAt    time.Time
	LastModified time.Time
	Depth        int
	Links        []url.URL
}

// Opts allow configuration of a Crawl.
type Opts struct {
	// Depth of search through linked pages.
	Depth int

	// List of regexp, matched urls will be excluded from crawling.
	Exclude []string

	// Follow links to external domains. Defaults to false.
	FollowExt bool

	// Number of concurrent workers scraping pages.
	NumWorkers int

	// Timeout for individual page fetches. Defaults to 5s.
	FetchTimeout time.Duration
}

// Merge Opts into one.
// Returns new Opts with non-zero values from params overwriting receiver values.
func (o Opts) Merge(other Opts) Opts {
	if other.Depth != 0 {
		o.Depth = other.Depth
	}

	if len(other.Exclude) != 0 {
		o.Exclude = other.Exclude
	}

	if other.NumWorkers != 0 {
		o.NumWorkers = other.NumWorkers
	}

	if other.FetchTimeout != 0 {
		o.FetchTimeout = other.FetchTimeout
	}

	o.FollowExt = other.FollowExt

	return o
}

// Crawl creates a web-crawler starting from a provided url.
type Crawl struct {
	BaseURL string
	Opts

	pl    sync.Mutex
	Pages []Page

	visited map[string]struct{}
	next    chan visit
	wg      sync.WaitGroup
}

// NewCrawl is a constructor for Crawl.
func NewCrawl(u string, opts Opts) (*Crawl, error) {
	if _, err := url.ParseRequestURI(u); err != nil {
		return nil, fmt.Errorf("%s is not a valid url", u)
	}

	opts = DefaultOpts.Merge(opts)

	return &Crawl{
		BaseURL: u,
		Opts:    opts,
		visited: make(map[string]struct{}),
	}, nil
}

// Go starts the crawl from the provided start url.
func (c *Crawl) Go(ctx context.Context) error {
	c.wg = sync.WaitGroup{}

	// buffer size is arbitrary but improves perf
	// by keeping ingest queue unrestricted
	c.next = make(chan visit, 100)

	// run workers to fetch queued urls
	workCtx, workCancel := context.WithCancel(ctx)
	defer workCancel()

	readyCh := make(chan chan visit, c.Opts.NumWorkers)

	log.Debugf("Crawl: starting %d fetch workers", c.Opts.NumWorkers)
	for i := 0; i < c.Opts.NumWorkers; i++ {
		go func(i int) {
			work := make(chan visit)

			for {
				if err := workCtx.Err(); err != nil {
					return
				}

				// signal that we're ready for work
				readyCh <- work

				// accept a job
				var visit visit
			wait:
				for {
					select {
					case <-workCtx.Done():
						return
					case visit = <-work:
						break wait
					}
				}

				// run a fetch
				log.Debugf("Worker %d: starting fetch for %s", i, visit.url)
				fetchCtx, cancel := context.WithTimeout(ctx, c.Opts.FetchTimeout)
				c.fetch(fetchCtx, visit.url, visit.depth)
				cancel()

				// mark it as completed
				c.wg.Done()
			}
		}(i)
	}

	// run a central dispatch loop
	go func() {
		for v := range c.next {
			// skip if we've already visited this page
			if _, ok := c.visited[v.url]; ok {
				c.wg.Done()
				continue
			}
			c.visited[v.url] = struct{}{}

			// we never want to block the url queue from being added to
			// so we use a short-lived goroutine here to handle the wait
			go func(v visit) {
				// wait for a ready worker
				workCh := <-readyCh

				// hand off the job
				workCh <- v
			}(v)
		}
	}()

	// trigger initial visit
	c.visit(c.BaseURL, 0)

	// wait for all work to complete
	waitCh := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(waitCh)
	}()

	// block until context timeout or completion
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
			close(c.next)
			return nil
		}
	}
}

func (c *Crawl) visit(url string, depth int) {
	c.wg.Add(1)
	c.next <- visit{url, depth}
}

func (c *Crawl) fetch(ctx context.Context, uri string, depth int) error {
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		log.Debugf("Skipping %s, error constructing http request: %s", uri, err)
		return err
	}

	req = req.WithContext(ctx)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Debugf("Skipping %s, error making http request: %s", uri, err)
		return err
	}

	if res.StatusCode != http.StatusOK {
		log.Debugf("Skipping %s, bad status code: %d", uri, res.StatusCode)
		return nil
	}

	if ok, _ := regexp.MatchString(`text\/html`, res.Header.Get("Content-Type")); !ok {
		log.Debugf("Skipping %s, not an HTML document", uri)
		return nil
	}

	page, err := NewPageFromResponse(res)
	if err != nil {
		log.Debugf("Skipping %s, could not parse page: %s", uri, err)
	}
	page.Depth = depth

	c.pl.Lock()
	c.Pages = append(c.Pages, page)
	c.pl.Unlock()

	if depth == c.Opts.Depth {
		return nil
	}

link:
	for _, link := range page.Links {
		// dont queue any urls in the exclusion list
		l := link.String()
		for _, exclusion := range c.Opts.Exclude {
			if ok, _ := regexp.MatchString(exclusion, l); ok {
				continue link
			}
		}

		if !c.Opts.FollowExt && link.Host != page.URL.Host {
			continue
		}

		c.visit(link.String(), depth+1)
	}

	return nil
}

// NewPageFromResponse is a helper func to construct a Page
// struct from an http.Response.
func NewPageFromResponse(res *http.Response) (Page, error) {
	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return Page{}, err
	}

	page := Page{
		URL:       *res.Request.URL,
		Title:     doc.Find("title").Text(),
		FetchedAt: time.Now(),
	}

	lm := res.Header.Get("Last-Modified")
	if lm != "" {
		page.LastModified, _ = time.Parse(http.TimeFormat, lm)
	}

	// extract links from document body
	seen := make(map[string]struct{})
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if !ok {
			return
		}

		hrefURL, err := url.Parse(href)
		if err != nil {
			return
		}

		// if the url has an extension and that extension
		// is not html then skip
		ext := filepath.Ext(hrefURL.Path)
		if ext != "" && ext != ".html" {
			return
		}

		// remove fragments and queries
		hrefURL.Fragment = ""
		hrefURL.RawQuery = ""

		// normalize: resolve the url relative to the base url to handle href="/page"
		// if the url href url is already absolute, this will be a no-op
		absURL := page.URL.ResolveReference(hrefURL)

		// normalize: remove trailing slash
		absURL.Path = strings.TrimRight(absURL.Path, "/")

		// skip over duplicate links
		if _, ok := seen[absURL.String()]; ok {
			return
		}
		seen[absURL.String()] = struct{}{}

		page.Links = append(page.Links, *absURL)
	})

	return page, nil
}

type visit struct {
	url   string
	depth int
}
