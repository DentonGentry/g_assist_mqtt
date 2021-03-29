package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"log"
	"net/http"
)

// -----------------------------------------------------------------------------

// https://developers.google.com/assistant/smarthome/reference/intent/sync
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

func GenerateSyncResponse(req IntentSyncRequest) IntentSyncResponse {
	var resp IntentSyncResponse
	resp.RequestId = req.RequestId
	resp.Payload.AgentUserId = AgentUserId
	for _, d := range devices {
		resp.Payload.Devices = append(resp.Payload.Devices, d.ToIntentSyncResponseDevice())
	}

	return resp
}

// -----------------------------------------------------------------------------

// https://developers.google.com/assistant/smarthome/reference/intent/query
type IntentQueryRequest struct {
	RequestId string `json:"requestId"`
	Inputs    []struct {
		Intent  string `json:"intent"`
		Payload struct {
			Devices []struct {
				Id         string      `json:"id"`
				CustomData interface{} `json:"customData,omitempty"`
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

func GenerateQueryResponse(req IntentQueryRequest) IntentQueryResponse {
	var resp IntentQueryResponse
	resp.RequestId = req.RequestId

	for _, d := range req.Inputs[0].Payload.Devices {
		DeviceQuery(d.Id, "")
	}

	return resp
}

// -----------------------------------------------------------------------------

// https://developers.google.com/assistant/smarthome/reference/intent/execute
type IntentExecuteRequest struct {
	RequestId string `json:"requestId"`
	Inputs    []struct {
		Intent  string `json:"intent"`
		Payload struct {
			Devices []struct {
				Id         string      `json:"id"`
				CustomData interface{} `json:"customData,omitempty"`
			} `json:"devices"`
			Execution []struct {
				Command string      `json:"command"`
				Params  interface{} `json:"params"`
			} `json:"execution"`
		} `json:"payload"`
	} `json:"inputs"`
}

// https://developers.google.com/assistant/smarthome/reference/intent/execute
type IntentExecuteResponse struct {
	RequestId string `json:"requestId"`
	Payload   struct {
		Commands []struct {
			Ids       []string    `json"ids"`
			Status    string      `json:"status"`
			States    interface{} `json:"states,omitempty"`
			ErrorCode string      `json:"errorCode,omitempty"`
		} `json:"commands"`
	} `json:"payload"`
}

func HandleFulfillment(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	ok, errorStr := ValidateJWT(r)
	if !ok {
		http.Error(w, errorStr, http.StatusUnauthorized)
		return
	}

	version, ok := r.Header["google-assistant-api-version"]
	if ok {
		if len(version) != 1 || version[0] != "v1" {
			errorStr = "500 error, unimplemented version: " + version[0]
			http.Error(w, errorStr, http.StatusInternalServerError)
			return
		}
	}

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "500 error, cannot read body", http.StatusInternalServerError)
	}
	log.Println("fulfillment: " + string(data))

	var sync IntentSyncRequest
	err = json.NewDecoder(bytes.NewReader(data)).Decode(&sync)
	if err == nil && len(sync.Inputs) == 1 && sync.Inputs[0].Intent == "action.devices.SYNC" {
		resp := GenerateSyncResponse(sync)
		w.Header().Set("Content-Type", "application/json")
		body, err := json.Marshal(resp)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			body = []byte("500 error, JSON serialization failed")
		}
		w.Write(body)
		return
	}

	var query IntentQueryRequest
	err = json.NewDecoder(bytes.NewReader(data)).Decode(&query)
	if err == nil && len(query.Inputs) > 0 && query.Inputs[0].Intent == "action.devices.QUERY" {
		resp := GenerateQueryResponse(query)
		w.Header().Set("Content-Type", "application/json")
		body, err := json.Marshal(resp)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			body = []byte("500 error, JSON serialization failed")
		}
		w.Write(body)
		return
	}
}
