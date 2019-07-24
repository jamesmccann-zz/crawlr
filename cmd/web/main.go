package main

import (
	"context"
	"flag"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

var (
	listenAddr = flag.String("addr", ":8080", "Listen address")
)

func init() {
	log.SetOutput(os.Stdout)
}

func main() {
	flag.Parse()

	exitCh := make(chan os.Signal)
	signal.Notify(exitCh, syscall.SIGINT, syscall.SIGTERM)

	handler, err := NewHandler()
	if err != nil {
		log.Fatalf("error initializing handler: %s", err)
	}

	srv := http.Server{
		Addr:    *listenAddr,
		Handler: handler.r,
	}

	go func() {
		log.Infof("Server listening on %s", *listenAddr)

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("error listening on %s: %s", *listenAddr, err)
		}
	}()

	<-exitCh

	ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
	srv.Shutdown(ctx)

	log.Infof("Server shutdown")
}
