package crawlr

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
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
	URL       string
	Title     string
	FetchedAt time.Time
	Depth     int
	Links     []string
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
	c.next = make(chan visit, 1000)

	// run workers to fetch queued urls
	workCtx, workCancel := context.WithCancel(ctx)
	defer workCancel()

	readyCh := make(chan chan visit, c.Opts.NumWorkers)

	for i := 0; i < c.Opts.NumWorkers; i++ {
		go func() {
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
				fetchCtx, cancel := context.WithTimeout(ctx, c.Opts.FetchTimeout)
				c.fetch(fetchCtx, visit.url, visit.depth)
				cancel()

				c.wg.Done()
			}
		}()
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

			// wait for a ready worker
			workCh := <-readyCh

			// hand off the job
			workCh <- v
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
			return nil
		}
	}

	return nil
}

func (c *Crawl) visit(url string, depth int) {
	c.wg.Add(1)
	c.next <- visit{url, depth}
}

func (c *Crawl) fetch(ctx context.Context, uri string, depth int) error {
	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return err
	}

	req = req.WithContext(ctx)

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	doc, err := goquery.NewDocumentFromReader(res.Body)
	if err != nil {
		return err
	}

	if res.StatusCode != http.StatusOK {
		return nil
	}

	if ok, _ := regexp.MatchString(`text\/html`, res.Header.Get("Content-Type")); !ok {
		return nil
	}

	pageURL, _ := url.Parse(uri)
	if err != nil {
		return err
	}

	page := Page{
		URL:       uri,
		Title:     doc.Find("title").Text(),
		FetchedAt: time.Now(),
		Depth:     depth,
	}

	seen := make(map[string]struct{})
	doc.Find("a[href]").Each(func(_ int, s *goquery.Selection) {
		href, ok := s.Attr("href")
		if !ok {
			return
		}

		// skip over any urls in the exclusion list
		for _, exclusion := range c.Opts.Exclude {
			if ok, _ := regexp.MatchString(exclusion, href); ok {
				return
			}
		}

		hrefURL, err := url.Parse(href)
		if err != nil {
			return
		}

		// remove fragments and queries
		hrefURL.Fragment = ""
		hrefURL.RawQuery = ""

		// normalize: resolve the url relative to the base url to handle href="/page"
		// if the url href url is already absolute, this will be a no-op
		absURL := pageURL.ResolveReference(hrefURL)
		if !c.Opts.FollowExt && absURL.Host != pageURL.Host {
			return
		}

		// normalize: remove trailing slash
		absURL.Path = strings.TrimRight(absURL.Path, "/")

		// skip over duplicate links
		if _, ok := seen[absURL.String()]; ok {
			return
		}
		seen[absURL.String()] = struct{}{}

		page.Links = append(page.Links, absURL.String())
	})

	c.pl.Lock()
	c.Pages = append(c.Pages, page)
	c.pl.Unlock()

	if depth == c.Opts.Depth {
		return nil
	}

	for _, link := range page.Links {
		c.visit(link, depth+1)
	}

	return nil
}

type visit struct {
	url   string
	depth int
}
