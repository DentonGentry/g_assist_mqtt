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

func GenerateSyncResponse(req IntentSyncRequest) ([]byte, error) {
	var resp IntentSyncResponse
	resp.RequestId = req.RequestId
	resp.Payload.AgentUserId = AgentUserId

	deviceLock.Lock()
	defer deviceLock.Unlock()
	for _, d := range devices {
		resp.Payload.Devices = append(resp.Payload.Devices, d.ToIntentSyncResponseDevice())
	}

	return json.Marshal(resp)
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

func GenerateQueryResponse(req IntentQueryRequest) ([]byte, error) {
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

	return json.Marshal(resp)
}

// -----------------------------------------------------------------------------

// https://developers.google.com/assistant/smarthome/reference/intent/execute
// Example:
// {"inputs":[
//	{"context":{"locale_country":"US","locale_language":"en"},
//	"intent":"action.devices.EXECUTE",
//	"payload":{
//	      "commands":[
//		    {"devices":[{"id":"840D8E5D7FCF"}],
//		"execution":[
//		{"command":"action.devices.commands.OnOff",
//		"params":{"on":false}}]}]}}],
//  "requestId":"3109023582895760782"}
type IntentExecuteRequest struct {
	RequestId string `json:"requestId"`
	Inputs    []struct {
		// Context isn't in Google's documentation but is present in requests.
		Context struct {
			LocaleCountry  string `json:"locale_country,omitempty"`
			LocaleLanguage string `json:"locale_language,omitempty"`
		} `json:"context"`
		Intent  string `json:"intent"`
		Payload struct {
			Commands []struct {
				Devices []struct {
					Id string `json:"id"`
				} `json:"devices"`
				Execution []struct {
					Command string `json:"command"`
					Params  struct {
						On bool `json:"on,omitempty"`
					} `json:"params"`
				} `json:"execution"`
			} `json:"commands"`
		} `json:"payload"`
	} `json:"inputs"`
}

// https://developers.google.com/assistant/smarthome/reference/intent/execute
// but supplemented with undocumented fields that Google sends like Context.
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
		Online bool `json:"online,omitempty"`
	} `json:"states,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}

func GenerateExecuteResponse(req IntentExecuteRequest) ([]byte, error) {
	var resp IntentExecuteResponse
	resp.RequestId = req.RequestId

	// No idea why the struct is defined so deeply nested. In practice, there has
	// only ever been one Input element, one Commands, and one Execution.
	for _, input := range req.Inputs {
		for _, command := range input.Payload.Commands {
			for _, device := range command.Devices {
				for _, execution := range command.Execution {
					var On bool
					if execution.Command == "action.devices.commands.OnOff" {
						if execution.Params.On {
							On = true
						} else {
							On = false
						}
					} else {
						var cmd IntentExecuteResponseCommand
						cmd.Ids = append(cmd.Ids, device.Id)
						cmd.Status = "ERROR"
						cmd.ErrorCode = "Command not supported"
						resp.Payload.Commands = append(resp.Payload.Commands, cmd)
						continue
					}

					deviceLock.Lock()
					var cmd IntentExecuteResponseCommand
					cmd.Ids = append(cmd.Ids, device.Id)
					d, ok := devices[device.Id]
					if !ok {
						cmd.Status = "OFFLINE"
						cmd.States.Online = false
					} else {
						d.SendPowerOnOff(On)
						cmd.Status = "ONLINE"
						cmd.States.On = On
						cmd.States.Online = true
					}
					resp.Payload.Commands = append(resp.Payload.Commands, cmd)
					deviceLock.Unlock()
				}
			}
		}
	}

	return json.Marshal(resp)
}

// -----------------------------------------------------------------------------

// A JSON struct with just the Intent populated, to figure out what it is. This happens
// to be identical to the v1 IntentSyncRequest, but we don't want to depend on that.
type IntentDecoder struct {
	RequestId string `json:"requestId"`
	Inputs    []struct {
		Intent string `json:"intent"`
	} `json:"inputs"`
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
	if ok && len(version) >= 1 {
		if version[0] != "v1" {
			errorStr = "500 error, unimplemented version: " + version[0]
			http.Error(w, errorStr, http.StatusInternalServerError)
			return
		}
	}

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "500 error, cannot read body", http.StatusInternalServerError)
		return
	}
	log.Println("fulfillment req: " + string(data))

	var intentStruct IntentDecoder
	err = json.NewDecoder(bytes.NewReader(data)).Decode(&intentStruct)
	if err != nil || len(intentStruct.Inputs) == 0 {
		http.Error(w, "No intent string", http.StatusBadRequest)
		return
	}
	if len(intentStruct.Inputs) > 1 {
		http.Error(w, "Only one Input is implemented", http.StatusNotImplemented)
		return
	}

	intent := intentStruct.Inputs[0].Intent
	var body []byte
	if intent == "action.devices.SYNC" {
		var sync IntentSyncRequest
		err = json.NewDecoder(bytes.NewReader(data)).Decode(&sync)
		if err != nil {
			http.Error(w, "Cannot decode SYNC", http.StatusBadRequest)
		}

		body, err = GenerateSyncResponse(sync)
	}

	if intent == "action.devices.QUERY" {
		var query IntentQueryRequest
		err = json.NewDecoder(bytes.NewReader(data)).Decode(&query)
		if err != nil {
			http.Error(w, "Cannot decode QUERY", http.StatusBadRequest)
		}

		body, err = GenerateQueryResponse(query)
	}
	if intent == "action.devices.EXECUTE" {
		var execute IntentExecuteRequest
		err = json.NewDecoder(bytes.NewReader(data)).Decode(&execute)
		if err != nil {
			http.Error(w, "Cannot decode EXECUTE", http.StatusBadRequest)
		}

		body, err = GenerateExecuteResponse(execute)
	}

	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		body = []byte("500 error, JSON serialization failed")
	}
	w.Header().Set("Content-Type", "application/json")
	log.Println("fulfillment response: " + string(body))
	w.Write(body)
	return
}
