package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

const AgentUserId = string("https://github.com/DentonGentry/g_assist_mqtt.git")

func HandleRoot(w http.ResponseWriter, r *http.Request) {
	// Access to '/' is not used in the actual appication, only for Google Cloud Run
	// checking if we're alive. We delay the response so that Cloud Run will let us
	// live a bit longer.
	time.Sleep(2 * time.Second)
}

func main() {
	// Google Cloud Run will populate PORT automatically.
	portStr := os.Getenv("PORT")
	if portStr == "" {
		portStr = "8080"
	}

	mux := http.NewServeMux()
	srv := &http.Server{
		Addr:         ":" + portStr,
		Handler:      mux,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
	}
	mux.HandleFunc("/quitquitquit", func(w http.ResponseWriter, r *http.Request) {
		srv.Shutdown(context.Background())
	})
	mux.HandleFunc("/debug", HandleDebug)
	mux.HandleFunc("/", HandleRoot)

	fmt.Println("Initializing fulfillment")
	mux.HandleFunc("/fulfillment", HandleFulfillment)

	fmt.Println("Starting MQTT client")
	go MQTT()

	fmt.Println("Initializing OAuth server")
	SetupOauth(mux)

	log.Fatal(srv.ListenAndServe())
}
