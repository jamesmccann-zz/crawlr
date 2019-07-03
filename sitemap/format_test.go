package sitemap

import (
	"testing"

	"github.com/jamesmccann/crawlr"
	"github.com/stretchr/testify/require"
)

func TestXmlFormatter_Format(t *testing.T) {
	t.Run("test outputs xml urlset", func(t *testing.T) {
		crawl := crawlr.Crawl{
			Pages: []crawlr.Page{
				{URL: "http://test.site/1"},
				{URL: "http://test.site/2"},
			},
		}

		output, err := XmlFormatter{}.Format(crawl)
		require.NoError(t, err)

		require.Equal(t, `<?xml version="1.0" encoding="UTF-8"?>
  <urlset schema="http://www.sitemaps.org/schemas/sitemap/0.9">
    <url>
      <loc>http://test.site/1</loc>
    </url>
    <url>
      <loc>http://test.site/2</loc>
    </url>
  </urlset>`, string(output))
	})
}
