package main

import (
	"encoding/json"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"os"
	"strings"
	"time"
)

// Subset of Intent SYNC which we will send back to Google.
// https://developers.google.com/assistant/smarthome/reference/intent/sync
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

// State extracted from tasmota/discovery/*/config events, used to construct
// a Smart Home Sync response
type TasmotaDevice struct {
	MacAddress   string
	IP           string
	FriendlyName string
	Hostname     string
	Hardware     string
	Software     string
	HasRelays    bool
	HasOnOff     bool
}

func (device *TasmotaDevice) ToIntentSyncResponseDevice() IntentSyncResponseDevice {
	var sync IntentSyncResponseDevice
	sync.Id = device.MacAddress

	if device.HasRelays {
		sync.Type = "action.devices.types.SWITCH"
	}
	if device.HasOnOff {
		sync.Traits = append(sync.Traits, "action.devices.traits.OnOff")
	}
	sync.Name.DefaultNames = append(sync.Name.DefaultNames, device.Hardware)
	sync.Name.Name = device.FriendlyName
	sync.WillReportState = false
	sync.DeviceInfo.Manufacturer = "Tasmota"
	sync.DeviceInfo.Model = device.Hardware
	sync.DeviceInfo.SwVersion = device.Software

	return sync
}

var devices = make(map[string]TasmotaDevice)

// Parse JSON received on tasmota/discovery/*/config
// {"ip":"10.1.10.100",
//  "dn":"Tasmota",
//  "fn":["ParentsRoomSwitch",null,null,null,null,null,null,null],
//  "hn":"parents-room-switch",
//  "mac":"BCDDC2000000",
//  "md":"MJ-S01 Switch",
//  "ty":0,
//  "if":0,
//  "ofln":"Offline",
//  "onln":"Online",
//  "state":["OFF","ON","TOGGLE","HOLD"],
//  "sw":"9.3.1",
//  "t":"parents-room-switch",
//  "ft":"%prefix%/%topic%/",
//  "tp":["cmnd","stat","tele"],
//  "rl":[1,0,0,0,0,0,0,0],
//  "swc":[-1,-1,-1,-1,-1,-1,-1,-1],
//  "swn":[null,null,null,null,null,null,null,null],
//  "btn":[0,0,0,0,0,0,0,0],
//  "so":{"4":0,"11":0,"13":0,"17":0,"20":0,"30":0,"68":0,"73":0,"82":0,"114":0,"117":0},
//  "lk":1,
//  "lt_st":0,
//  "sho":[0,0,0,0],
//  "ver":1}
func ParseDeviceDiscovery(jsonStr []byte) (TasmotaDevice, error) {
	var device TasmotaDevice

	jsonMap := make(map[string]interface{})
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		return device, err
	}

	device.MacAddress = jsonMap["mac"].(string)
	device.IP = jsonMap["ip"].(string)
	fn := jsonMap["fn"].([]interface{})
	device.FriendlyName = fn[0].(string)
	device.Hostname = jsonMap["hn"].(string)
	device.Hardware = jsonMap["md"].(string)
	device.Software = jsonMap["sw"].(string)

	relays := jsonMap["rl"].([]interface{})
	for _, r := range relays {
		if r != 0 {
			device.HasRelays = true
		}
	}

	state := jsonMap["state"].([]interface{})
	for _, s := range state {
		item := s.(string)
		if item == "OFF" || item == "ON" {
			device.HasOnOff = true
		}
	}

	return device, nil
}

var messagePubHandler mqtt.MessageHandler = func(client mqtt.Client, msg mqtt.Message) {
	t := strings.Split(msg.Topic(), "/")
	if len(t) == 4 && t[0] == "tasmota" && t[1] == "discovery" && t[3] == "config" {
		address := t[2]
		device, err := ParseDeviceDiscovery(msg.Payload())
		if err == nil {
			devices[address] = device
		}
	}
}

var connectHandler mqtt.OnConnectHandler = func(client mqtt.Client) {
	fmt.Println("Connected")
}

var connectLostHandler mqtt.ConnectionLostHandler = func(client mqtt.Client, err error) {
	fmt.Printf("Connect lost: %v", err)
}

func SubscribeMQTT() error {
	// TODO: broker+port need to come from the environment.
	var broker = "100.126.243.58"
	var port = 1883
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("mqtt://%s:%d", broker, port))
	opts.SetClientID("Google Assistant MQTT connector")
	opts.SetUsername(os.Getenv("MQTT_USERNAME"))
	opts.SetPassword(os.Getenv("MQTT_PASSWORD"))
	opts.SetDefaultPublishHandler(messagePubHandler)
	opts.OnConnect = connectHandler
	opts.OnConnectionLost = connectLostHandler
	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.Wait()
	if token.Error() != nil {
		return token.Error()
	}

	topic := "tasmota/discovery/#"
	token = client.Subscribe(topic, 1, nil)
	token.Wait()
	if token.Error() != nil {
		return token.Error()
	}

	fmt.Printf("Subscribed to topic: %s\n", topic)

	time.Sleep(5 * time.Second)
	fmt.Println("MQTT Devices:")
	for _, d := range devices {
		fmt.Println(d)
	}

	return nil
}

func SetupMQTT() {
	for SubscribeMQTT() != nil {
		time.Sleep(1 * time.Second)
	}
}
