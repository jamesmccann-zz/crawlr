package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"github.com/google/uuid"
	"github.com/julienschmidt/httprouter"
	log "github.com/sirupsen/logrus"

	"github.com/jamesmccann/crawlr"
	"github.com/jamesmccann/crawlr/sitemap"
)

type Queuer interface {
	Queue(url string) string
}

type Storage interface {
	Put(string, *crawlr.Crawl) error
	Get(string) (*crawlr.Crawl, error)
}

type Handler struct {
	r *httprouter.Router
	s Storage
}

func NewHandler() (*Handler, error) {
	router := httprouter.New()

	h := &Handler{
		r: router,
		s: &InMemStore{
			storage: make(map[string]*crawlr.Crawl),
		},
	}

	router.POST("/crawl", h.queue)
	router.GET("/crawl/:id", h.get)
	router.GET("/health", health)

	return h, nil
}

func health(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	fmt.Fprintln(w, "OK")
}

func (h *Handler) queue(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	req := struct {
		URL string `json:"url"`
	}{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"json is an invalid format"}`, http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, `{"error":"url is a required parameter"}`, http.StatusBadRequest)
		return
	}

	if _, err := url.Parse(req.URL); err != nil {
		http.Error(w, `{"error":"url is not a valid format"}`, http.StatusBadRequest)
	}

	id := uuid.New()

	resp := struct {
		ID string `json:"id"`
	}{id.String()}

	w.WriteHeader(http.StatusCreated)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Errorf("handler/queue: error encoding response json: %s", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	go func() {
		crawl, err := crawlr.NewCrawl(req.URL, crawlr.DefaultOpts)
		if err != nil {
			// TODO write an error status for the crawl
			return
		}

		if err := crawl.Go(r.Context()); err != nil {
			// TODO write an error status for the crawl
			return
		}

		h.s.Put(id.String(), crawl)
	}()
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	id := ps.ByName("id")

	crawl, err := h.s.Get(id)
	switch {
	case err == ErrCrawlNotFound:
		w.WriteHeader(http.StatusNotFound)
		return
	case err != nil:
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	formatter := sitemap.Formatters["simple"]
	sitemap, err := formatter.Format(*crawl)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	fmt.Fprint(w, string(sitemap))
}

var ErrCrawlNotFound = errors.New("crawl not found")

type InMemStore struct {
	sl      sync.Mutex
	storage map[string]*crawlr.Crawl
}

func (s *InMemStore) Get(id string) (*crawlr.Crawl, error) {
	s.sl.Lock()
	crawl, ok := s.storage[id]
	s.sl.Unlock()

	if !ok {
		return nil, ErrCrawlNotFound
	}

	return crawl, nil
}

func (s *InMemStore) Put(id string, crawl *crawlr.Crawl) error {
	s.sl.Lock()
	s.storage[id] = crawl
	s.sl.Unlock()
	return nil
}
