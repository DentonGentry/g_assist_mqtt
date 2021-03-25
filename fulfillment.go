package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type IntentSyncRequest struct {
	RequestId string `json:"requestId"`
	Inputs    []struct {
		Intent string `json:"intent"`
	} `json:"inputs"`
}

type IntentSyncResponse struct {
	RequestId string `json:"requestId"`
	Payload   struct {
		AgentUserId string                     `json:"agentUserId"`
		Devices     []IntentSyncResponseDevice `json:"devices"`
	} `json:"payload"`
}

// https://developers.google.com/assistant/smarthome/reference/intent/query
type IntentQueryRequest struct {
	RequestId string `json:"requestId"`
	Inputs    []struct {
		Intent  string `json:"intent"`
		Payload struct {
			Devices []struct {
				Id         string      `json:"id"`
				CustomData interface{} `json:"customData"`
			} `json:"devices"`
		} `json:"payload"`
	} `json:"inputs"`
}

// https://developers.google.com/assistant/smarthome/reference/intent/query
type IntentQueryResponse struct {
	RequestId string `json:"requestId"`
	Payload   struct {
		ErrorCode   string `json:"errorCode,omitempty"`
		DebugString string `json:"debugString,omitempty"`
		Devices     []struct {
			Id     string `json"id"`
			Online bool   `json:"online"`
			Status string `json:"status"`
		} `json:"devices"`
	} `json:"payload"`
}

func GenerateSyncResponse(req IntentSyncRequest) IntentSyncResponse {
	var resp IntentSyncResponse
	resp.RequestId = req.RequestId
	resp.Payload.AgentUserId = "my-app"
	for _, d := range devices {
		resp.Payload.Devices = append(resp.Payload.Devices, d.ToIntentSyncResponseDevice())
	}

	return resp
}

func HandleFulfillment(w http.ResponseWriter, r *http.Request) {
	// TODO: check JWT token here.

	version, ok := r.Header["google-assistant-api-version"]
	if ok {
		if len(version) != 1 || version[0] != "v1" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("500 error, unimplemented version: " + version[0]))
		}
	}

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var sync IntentSyncRequest
	err := dec.Decode(&sync)
	fmt.Printf("devices = %d\n", len(devices))
	if err == nil && len(sync.Inputs) == 1 && sync.Inputs[0].Intent == "action.devices.SYNC" {
		resp := GenerateSyncResponse(sync)
		w.Header().Set("Content-Type", "application/json")
		body, err := json.Marshal(resp)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			body = []byte("500 error, JSON serialization failed")
		}
		w.Write(body)
	}
}
