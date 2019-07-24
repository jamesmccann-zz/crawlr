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
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
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
		url := xmlUrl{Loc: page.URL.String()}
		if !page.LastModified.IsZero() {
			url.LastMod = page.LastModified.Format("2006-01-02")
		}

		sitemap.Urls = append(sitemap.Urls, url)
	}

	output, err := xml.MarshalIndent(sitemap, "  ", "  ")
	if err != nil {
		return nil, fmt.Errorf("error formatting sitemap as xml: %s", err)
	}

	return []byte(xml.Header + string(output)), nil
}

type SimpleFormatter struct{}

func (_ SimpleFormatter) Format(crawl crawlr.Crawl) ([]byte, error) {
	var buf bytes.Buffer
	seen := make(map[string]struct{})

	buf.WriteString(fmt.Sprintf("%s\n", crawl.BaseURL.String()))
	for _, page := range crawl.Pages[1:] {
		buf.WriteString(fmt.Sprintf("%s- %s\n", "  ", page.URL.String()))
		buf.WriteString(fmt.Sprintf("%s  Last modified: %s\n", "  ", page.LastModified))
		seen[page.URL.String()] = struct{}{}

		for _, link := range page.Links {
			if _, ok := seen[link.String()]; ok {
				continue
			}
			seen[link.String()] = struct{}{}

			buf.WriteString(fmt.Sprintf("%s- %s\n", "    ", link.String()))
		}
	}

	return buf.Bytes(), nil
}
