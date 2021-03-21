package main

import (
	"context"
	//"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func HandleRoot(w http.ResponseWriter, r *http.Request) {
	// Access to '/' is not used in the actual appication, only for Google Cloud Run checking
	// if we're alive. We delay the response so that Cloud Run will keep us active for a while.
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
	mux.HandleFunc("/", HandleRoot)

	fmt.Println("Initializing fulfillment")
	mux.HandleFunc("/fulfillment", HandleFulfillment)

	fmt.Println("Starting MQTT client")
	go SetupMQTT()

	fmt.Println("Initializing OAuth server")
	SetupOauth(mux)

	//time.Sleep(5 * time.Second)
	//for address, d := range devices {
	//	b, err := json.MarshalIndent(d.ToIntentSyncResponse(), "", "  ")
	//	if err != nil {
	//		panic("json.Marshal failed")
	//	}
	//	fmt.Printf("%s = %s\n", address, string(b))
	//}

	log.Fatal(srv.ListenAndServe())
}
