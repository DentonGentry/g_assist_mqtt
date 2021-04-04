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

// Subset of Intent SYNC which we will send back to Google.
type IntentSyncResponseDevice struct {
	Id     string   `json:"id"`
	Type   string   `json:"type"`
	Traits []string `json:"traits"`
	Name   struct {
		DefaultNames []string `json:"defaultNames"`
		Name         string   `json:"name"`
	} `json:"name"`
	WillReportState bool `json:"willReportState"`
	DeviceInfo      struct {
		Manufacturer string `json:"manufacturer,omitempty"`
		Model        string `json:"model,omitempty"`
		SwVersion    string `json:"swVersion,omitempty"`
	} `json:"deviceInfo,omitempty"`
}

func GenerateSyncResponse(req IntentSyncRequest) IntentSyncResponse {
	var resp IntentSyncResponse
	resp.RequestId = req.RequestId
	resp.Payload.AgentUserId = AgentUserId

	deviceLock.Lock()
	defer deviceLock.Unlock()
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
				Id string `json:"id"`
			} `json:"devices"`
		} `json:"payload"`
	} `json:"inputs"`
}

// https://developers.google.com/assistant/smarthome/reference/intent/query
type IntentQueryResponse struct {
	RequestId string `json:"requestId"`
	Payload   struct {
		ErrorCode   string                      `json:"errorCode,omitempty"`
		DebugString string                      `json:"debugString,omitempty"`
		Devices     []IntentQueryResponseDevice `json:"devices"`
	} `json:"payload"`
}

type IntentQueryResponseDevice struct {
	Id     string `json:"id"`
	Online bool   `json:"online"`
	Status string `json:"status"`
	On     bool   `json:"on,omitempty"`
}

func GenerateQueryResponse(req IntentQueryRequest) IntentQueryResponse {
	var resp IntentQueryResponse
	resp.RequestId = req.RequestId

	deviceLock.Lock()
	defer deviceLock.Unlock()
	for _, q := range req.Inputs[0].Payload.Devices {
		d, ok := devices[q.Id]
		if !ok {
			var offline IntentQueryResponseDevice
			offline.Id = q.Id
			offline.Online = false
			offline.Status = "OFFLINE"
			resp.Payload.Devices = append(resp.Payload.Devices, offline)
		} else {
			resp.Payload.Devices = append(resp.Payload.Devices,
				d.ToIntentQueryResponseDevice())
		}
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
				Id string `json:"id"`
			} `json:"devices"`
			Execution []struct {
				Command string `json:"command"`
				Params  struct {
					On bool `json:"on,omitempty"`
				} `json:"params"`
			} `json:"execution"`
		} `json:"payload"`
	} `json:"inputs"`
}

// https://developers.google.com/assistant/smarthome/reference/intent/execute
type IntentExecuteResponse struct {
	RequestId string `json:"requestId"`
	Payload   struct {
		ErrorCode   string                         `json:"errorCode,omitempty"`
		DebugString string                         `json:"debugString,omitempty"`
		Commands    []IntentExecuteResponseCommand `json:"commands"`
	} `json:"payload"`
}

type IntentExecuteResponseCommand struct {
	Ids    []string `json:"ids"`
	Status string   `json:"status"`
	States struct {
		On     bool `json:"on,omitempty"`
		Online bool `json:"online"`
	} `json:"states,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}

func GenerateExecuteResponse(req IntentExecuteRequest) IntentExecuteResponse {
	var resp IntentExecuteResponse
	resp.RequestId = req.RequestId

	if len(req.Inputs[0].Payload.Execution) != 1 {
		resp.Payload.ErrorCode = "Only one Execute block is implemented"
		return resp
	}

	exe := req.Inputs[0].Payload.Execution[0]
	var On bool
	if exe.Command == "action.devices.commands.OnOff" {
		if exe.Params.On {
			On = true
		} else {
			On = false
		}
	} else {
		resp.Payload.ErrorCode = "Only OnOff implemented"
		return resp
	}

	deviceLock.Lock()
	defer deviceLock.Unlock()
	for _, x := range req.Inputs[0].Payload.Devices {
		var cmd IntentExecuteResponseCommand
		cmd.Ids = append(cmd.Ids, x.Id)
		d, ok := devices[x.Id]
		if !ok {
			cmd.Status = "OFFLINE"
			cmd.States.Online = false
		} else {
			d.SendExecute(On)
			cmd.Status = "ONLINE"
			cmd.States.On = On
			cmd.States.Online = true
		}
		resp.Payload.Commands = append(resp.Payload.Commands, cmd)
	}

	return resp
}

// -----------------------------------------------------------------------------

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
	log.Println("fulfillment req: " + string(data))

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
		log.Println("fulfillment SYNC: " + string(body))
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
		log.Println("fulfillment QUERY: " + string(body))
		w.Write(body)
		return
	}

	var execute IntentExecuteRequest
	err = json.NewDecoder(bytes.NewReader(data)).Decode(&query)
	if err == nil && len(execute.Inputs) > 0 && execute.Inputs[0].Intent == "action.devices.EXECUTE" {
		resp := GenerateExecuteResponse(execute)
		w.Header().Set("Content-Type", "application/json")
		body, err := json.Marshal(resp)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			body = []byte("500 error, JSON serialization failed")
		}
		log.Println("fulfillment EXECUTE: " + string(body))
		w.Write(body)
		return
	}
}
