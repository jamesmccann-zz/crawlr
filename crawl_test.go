package crawlr

import (
	"context"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCrawl_Go(t *testing.T) {
	t.Run("test crawl collects page objects for fetched webpage", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp, err := ioutil.ReadFile("./testdata/1_no_links.html")
			require.NoError(t, err)

			fmt.Fprintln(w, string(resp))
		}))

		crawl, err := NewCrawl(ts.URL, DefaultOpts)
		require.NoError(t, err)

		err = crawl.Go(context.TODO())
		require.NoError(t, err)

		require.Equal(t, 1, len(crawl.Pages))
		require.Equal(t, "Test page", crawl.Pages[0].Title)
		require.Equal(t, ts.URL, crawl.Pages[0].URL.String())
		require.Equal(t, 0, crawl.Results[0].Depth)
		require.WithinDuration(t, time.Now(), crawl.Pages[0].FetchedAt, time.Millisecond)
	})

	t.Run("test crawl visits nested webpages", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp, err := ioutil.ReadFile("./testdata/2_multiple_links.html")
			require.NoError(t, err)

			fmt.Fprintln(w, string(resp))
		}))

		crawl, err := NewCrawl(ts.URL, DefaultOpts)
		require.Nil(t, err)

		err = crawl.Go(context.TODO())
		require.Nil(t, err)

		require.Equal(t, 5, len(crawl.Pages))
		require.True(t, containsUrl(crawl, ts.URL))
		require.True(t, containsUrl(crawl, ts.URL+"/about"))
		require.True(t, containsUrl(crawl, ts.URL+"/food"))
		require.True(t, containsUrl(crawl, ts.URL+"/blog"))
		require.True(t, containsUrl(crawl, ts.URL+"/contact"))
		require.Equal(t, 1, crawl.Results[len(crawl.Results)-1].Depth)
	})

	t.Run("test does not re-crawl visited pages", func(t *testing.T) {
		var visited int
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			visited = visited + 1

			resp, err := ioutil.ReadFile("./testdata/3_multiple_same_links.html")
			require.NoError(t, err)

			fmt.Fprintln(w, string(resp))
		}))

		crawl, err := NewCrawl(ts.URL, DefaultOpts)
		require.Nil(t, err)

		err = crawl.Go(context.TODO())
		require.Nil(t, err)

		// only 2 unique pages - root and /about
		require.Equal(t, 2, visited)
	})

	t.Run("test does not exceed depth specified in opts", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/blog" {
				resp, err := ioutil.ReadFile("./testdata/4_blog_page.html")
				require.NoError(t, err)
				fmt.Fprintln(w, string(resp))
				return
			}

			resp, err := ioutil.ReadFile("./testdata/2_multiple_links.html")
			require.NoError(t, err)
			fmt.Fprintln(w, string(resp))
		}))

		crawl, err := NewCrawl(ts.URL, DefaultOpts.Merge(Opts{Depth: 1}))
		require.Nil(t, err)

		err = crawl.Go(context.TODO())
		require.Nil(t, err)

		// we should not have fetched /page-nested-under-blog
		require.Equal(t, 5, len(crawl.Pages))
		require.False(t, containsUrl(crawl, ts.URL+"/page-nested-under-blog"))
	})

	t.Run("test does not follow external urls via flag in opts", func(t *testing.T) {
		ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp, err := ioutil.ReadFile("./testdata/1_no_links.html")
			require.NoError(t, err)
			fmt.Fprintln(w, string(resp))
		}))

		ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, `
<html>
<body>
  <a href="`+ts2.URL+`">External link</a>
</body>
</html>
			`)
		}))

		crawl, err := NewCrawl(ts1.URL, DefaultOpts.Merge(Opts{FollowExt: false}))
		require.Nil(t, err)

		err = crawl.Go(context.TODO())
		require.Nil(t, err)

		require.Equal(t, 1, len(crawl.Pages))
		require.False(t, containsUrl(crawl, ts2.URL))
	})

	t.Run("test does fetch external urls via flag in opts", func(t *testing.T) {
		ts2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp, err := ioutil.ReadFile("./testdata/1_no_links.html")
			require.NoError(t, err)
			fmt.Fprintln(w, string(resp))
		}))

		ts1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintln(w, `
<html>
<body>
  <a href="`+ts2.URL+`">External link</a>
</body>
</html>
			`)
		}))

		crawl, err := NewCrawl(ts1.URL, DefaultOpts.Merge(Opts{FollowExt: true}))
		require.Nil(t, err)

		err = crawl.Go(context.TODO())
		require.Nil(t, err)

		require.Equal(t, 2, len(crawl.Pages))
		require.True(t, containsUrl(crawl, ts2.URL))
	})

	t.Run("test excludes links matching exclude regexp in opts", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp, err := ioutil.ReadFile("./testdata/2_multiple_links.html")
			require.NoError(t, err)

			fmt.Fprintln(w, string(resp))
		}))

		crawl, err := NewCrawl(ts.URL, DefaultOpts.Merge(Opts{
			Exclude: []string{
				"blog",
			},
		}))
		require.Nil(t, err)

		err = crawl.Go(context.TODO())
		require.Nil(t, err)

		require.Equal(t, 4, len(crawl.Pages))
		require.True(t, containsUrl(crawl, ts.URL))
		require.True(t, containsUrl(crawl, ts.URL+"/about"))
		require.True(t, containsUrl(crawl, ts.URL+"/food"))
		require.False(t, containsUrl(crawl, ts.URL+"/blog"))
		require.True(t, containsUrl(crawl, ts.URL+"/contact"))
	})
}

func containsUrl(crawl *Crawl, url string) bool {
	for _, page := range crawl.Pages {
		if page.URL.String() == url {
			return true
		}
	}

	return false
}
