package sitemap

import (
	"bytes"
	"encoding/xml"
	"fmt"

	"github.com/jamesmccann/crawlr"
)

var Formatters = map[string]Formatter{
	"xml":    XmlFormatter{},
	"simple": SimpleFormatter{},
}

type Formatter interface {
	Format(crawl crawlr.Crawl) ([]byte, error)
}

type xmlUrl struct {
	Loc string `xml:"loc"`
}

type urlset struct {
	Schema string   `xml:"schema,attr"`
	Urls   []xmlUrl `xml:"url"`
}

type XmlFormatter struct{}

func (_ XmlFormatter) Format(crawl crawlr.Crawl) ([]byte, error) {
	sitemap := urlset{
		Schema: "http://www.sitemaps.org/schemas/sitemap/0.9",
	}

	for _, page := range crawl.Pages {
		sitemap.Urls = append(sitemap.Urls, xmlUrl{
			Loc: page.URL,
		})
	}

	output, err := xml.MarshalIndent(sitemap, "  ", "  ")
	if err != nil {
		return nil, fmt.Errorf("error formatting sitemap as xml: %s", err)
	}

	return []byte(xml.Header + string(output)), nil
}

type SimpleFormatter struct{}

func (_ SimpleFormatter) Format(crawl crawlr.Crawl) ([]byte, error) {
	// start with base url
	var buf bytes.Buffer
	buf.WriteString("Crawl results for " + crawl.Pages[0].URL + "\n")

	seen := make(map[string]struct{})
	for _, page := range crawl.Pages[1:] {
		buf.WriteString(fmt.Sprintf("%s- %s\n", "  ", page.URL))
		seen[page.URL] = struct{}{}

		for _, link := range page.Links {
			if _, ok := seen[link]; ok {
				continue
			}
			seen[link] = struct{}{}

			buf.WriteString(fmt.Sprintf("%s- %s\n", "    ", link))
		}
	}

	return buf.Bytes(), nil
}
