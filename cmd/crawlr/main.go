package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/jamesmccann/crawlr"
	"github.com/jamesmccann/crawlr/sitemap"
	log "github.com/sirupsen/logrus"
)

var (
	depth      = flag.Int("d", 1, "Search depth. Set to -1 to crawl all pages reachable from the initial page.")
	format     = flag.String("f", "xml", "Output sitemap format (xml|simple).")
	concurrent = flag.Int("c", 1, "Number of concurrent workers for crawling.")
	exclude    = flag.String("exclude", "", "Comma-separated list of regexp for urls to exclude from crawling.")
	verbose    = flag.Bool("v", false, "Enables verbose debug logging.")
	help       = flag.Bool("h", false, "Prints this help message.")
)

func init() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: crawlr [options] <url>\n")
		flag.PrintDefaults()
	}

	log.SetOutput(os.Stdout)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Print("Error: start url is required\n")
		os.Exit(1)
	}

	flag.Parse()
	if *help {
		flag.Usage()
		os.Exit(0)
	}

	if *verbose {
		log.SetLevel(log.DebugLevel)
	}

	if _, ok := sitemap.Formatters[*format]; !ok {
		fmt.Printf("Error: format %s not recognized, supported: (xml|simple)\n", *format)
		os.Exit(1)
	}

	excludes := []string{}
	if *exclude != "" {
		excludes = strings.Split(*exclude, ",")
	}

	opts := crawlr.DefaultOpts.Merge(crawlr.Opts{
		Depth:      *depth,
		NumWorkers: *concurrent,
		Exclude:    excludes,
	})

	crawl, err := crawlr.NewCrawl(os.Args[len(os.Args)-1], opts)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	exitCh := make(chan os.Signal)
	signal.Notify(exitCh, syscall.SIGINT, syscall.SIGTERM)

	runCtx, cancel := context.WithCancel(context.Background())
	go func() {
		<-exitCh
		cancel()
	}()

	err = crawl.Go(runCtx)
	switch {
	case runCtx.Err() != nil:
		// user cancelled
		os.Exit(0)
	case err != nil:
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Crawl results for %s:\n", crawl.Pages[0].URL.String())
	fmt.Printf("%d links crawled, %d pages fetched, %d skipped, %d failed\n",
		crawl.NumCrawled(), crawl.NumFetched(), crawl.NumSkipped(), crawl.NumFailed())

	formatter := sitemap.Formatters[*format]
	sitemap, err := formatter.Format(*crawl)
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		os.Exit(1)
	}
	fmt.Println(string(sitemap))
}
