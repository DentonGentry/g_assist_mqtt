package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"
)

func HandleFulfillment(w http.ResponseWriter, r *http.Request) {

}

func FulfillmentServer() {
	// Google Cloud Run will populate PORT automatically.
	portStr := os.Getenv("PORT")
	if portStr == "" {
		portStr = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/fulfillment", HandleFulfillment)
	srv := &http.Server{
		Addr:         ":" + portStr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	mux.HandleFunc("/quitquitquit", func(w http.ResponseWriter, r *http.Request) {
		srv.Shutdown(context.Background())
	})

	log.Fatal(srv.ListenAndServe())
}
