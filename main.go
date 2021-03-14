package main

import (
	"encoding/json"
	"fmt"
	"time"
)

func main() {
	// starts a number of goroutines listening for updates from MQTT
	SetupMQTT()

	time.Sleep(5 * time.Second)
	for address, d := range devices {
		b, err := json.MarshalIndent(d.ToIntentSyncResponse(), "", "  ")
		if err != nil {
			panic("json.Marshal failed")
		}
		fmt.Printf("%s = %s\n", address, string(b))
	}

	go FulfillmentServer()
	go OauthServer()

	time.Sleep(1 * time.Hour)
}
