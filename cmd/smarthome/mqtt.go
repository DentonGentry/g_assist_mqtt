package main

import (
	"encoding/json"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// MQTT QoS values
	AtMostOnce  = 0
	AtLeastOnce = 1
	ExactlyOnce = 2
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
	MacAddress    string
	IP            string
	FriendlyName  string
	Hostname      string
	Hardware      string
	Software      string
	HasRelays     bool
	HasOnOff      bool
	TopicName     string
	TopicPrefixes []string
}

var devices = make(map[string]TasmotaDevice)

// Produce the Device portion of a Google Smart Home Sync Response
// https://developers.google.com/assistant/smarthome/reference/intent/sync
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
// as described in https://github.com/arendst/Tasmota/issues/9267
func ParseTasmotaDiscovery(jsonStr []byte) (TasmotaDevice, error) {
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

	device.TopicName = jsonMap["t"].(string)
	for _, i := range jsonMap["tp"].([]interface{}) {
		prefix := i.(string)
		device.TopicPrefixes = append(device.TopicPrefixes, prefix)
	}

	return device, nil
}

// Call for MQTT messages arriving on /tasmota/discovery/#
func TasmotaDiscoveryMessageHandler(client mqtt.Client, msg mqtt.Message) {
	t := strings.Split(msg.Topic(), "/")
	if len(t) != 4 || t[0] != "tasmota" || t[1] != "discovery" {
		log.Println("TasmotaDiscovery unknown topic: " + msg.Topic())
		return
	}

	address := t[2]
	if t[3] != "config" {
		// we don't process "/tasmota/discovery/*/sensors" yet
		return
	}

	device, err := ParseTasmotaDiscovery(msg.Payload())
	if err == nil {
		devices[address] = device

		// Subscribe to the device's topics right away
		topics := make(map[string]byte)
		for _, prefix := range device.TopicPrefixes {
			if prefix != "stat" && prefix != "tele" {
				continue // no need to subscribe to command channel etc
			}
			topic := "/" + prefix + "/" + device.TopicName + "/#"
			topics[topic] = AtLeastOnce
		}
		token := client.SubscribeMultiple(topics,
			func(client mqtt.Client, msg mqtt.Message) {
				m := SerializedRcvMsg{"StateMessageHandler", msg}
				serializedRcvMsgCh <- m
			})
		go func() {
			_ = token.Wait()
			if token.Error() != nil {
				log.Printf("subscribe %s=%v", device.TopicName, token.Error())
			}
		}()
	}
}

// handles /stat/device-topic/RESULT and /tele/device-topic/STATE messages
// serialized through SerializeDevicesFunc
//
// Example (both topics send the same message format):
// {"Time":"2021-03-28T14:46:16","Uptime":"21T16:41:40","UptimeSec":1874500,"Heap":29,
//  "SleepMode":"Dynamic","Sleep":50,"LoadAvg":19,"MqttCount":20,"POWER":"OFF",
//  "Wifi":{"AP":2,"SSId":"MY-SSID","BSSId":"00:11:22:33:44:55","Channel":1,"RSSI":44,
//          "Signal":-78,"LinkCount":17,"Downtime":"0T00:05:18"}}
func StateMessageHandler(client mqtt.Client, msg mqtt.Message) {
	t := strings.Split(msg.Topic(), "/")
	if len(t) < 3 {
		log.Println("MQTT State handler: unknown topic: " + msg.Topic())
		return
	}
	log.Println("MQTT State handler: " + msg.Topic())
}

type SerializedRcvMsg struct {
	handler string
	msg     mqtt.Message
}
type SerializedQuery struct {
	Id string
}

var serializedRcvMsgCh chan SerializedRcvMsg
var serializedQueryCh chan SerializedQuery

// MQTT uses a goroutine per incoming message. Handling anything which needs to
// access the devices map would either require locking, or to serialize reception
// to one goroutine. We've chosen to do that: this routine pulls messages from
// a channel to be processwed one by one.
func SerializeDevicesFunc(client mqtt.Client) {
	for {
		select {
		case m := <-serializedRcvMsgCh:
			serializeReceive(client, m)
		case q := <-serializedQueryCh:
			serializePublish(client, q)
		}
	}
}

func serializeReceive(client mqtt.Client, m SerializedRcvMsg) {
	if m.handler == "StateMessageHandler" {
		StateMessageHandler(client, m.msg)
	} else if m.handler == "TasmotaDiscoveryMessageHandler" {
		TasmotaDiscoveryMessageHandler(client, m.msg)
	}
}

func serializePublish(client mqtt.Client, q SerializedQuery) {
	d, ok := devices[q.Id]
	if !ok {
		log.Println("serializePublish: unknown Device: " + q.Id)
		return
	}

	topic := "/cmnd/" + d.TopicName + "/STATE"
	retained := false
	log.Println("Publishing to " + topic)
	token := client.Publish(topic, AtLeastOnce, retained, "")
	go func() {
		_ = token.Wait()
		if token.Error() != nil {
			return
		}
	}()
}

func DeviceQuery(Id string, Text string) {
	q := SerializedQuery{Id}
	serializedQueryCh <- q
}

func DefaultMessageHandler(client mqtt.Client, msg mqtt.Message) {
	log.Println("DefaultMessageHandler unexpected topic: " + msg.Topic())
}

func OnConnectHandler(client mqtt.Client) {
	log.Println("MQTT Connected")
}

func ConnectionLostHandler(client mqtt.Client, err error) {
	log.Printf("MQTT connection lost: %v", err)
}

func ConnectToMQTT() (client mqtt.Client, err error) {
	opts := mqtt.NewClientOptions()
	broker := "mqtt://" + os.Getenv("MQTT_IP_ADDR") + ":" + os.Getenv("MQTT_PORT")
	opts.AddBroker(broker)
	// Only one client with the same ID can connect, add a random slug at end
	opts.SetClientID(AgentUserId + ":" + strconv.FormatUint(rand.Uint64(), 32))
	opts.SetOrderMatters(false)
	opts.SetUsername(os.Getenv("MQTT_USERNAME"))
	opts.SetPassword(os.Getenv("MQTT_PASSWORD"))
	opts.SetDefaultPublishHandler(DefaultMessageHandler)
	opts.OnConnect = OnConnectHandler
	opts.OnConnectionLost = ConnectionLostHandler
	client = mqtt.NewClient(opts)
	token := client.Connect()
	token.Wait()
	if token.Error() != nil {
		return client, token.Error()
	}

	return client, nil
}

func MQTT() {
	mqtt.ERROR = log.New(os.Stdout, "[MQTT ERROR] ", 0)
	mqtt.CRITICAL = log.New(os.Stdout, "[MQTT CRIT] ", 0)
	mqtt.WARN = log.New(os.Stdout, "[MQTT WARN]  ", 0)
	//mqtt.DEBUG = log.New(os.Stdout, "[MQTT DEBUG] ", 0) // quite verbose

	var client mqtt.Client
	var err error
	for client, err = ConnectToMQTT(); err != nil; {
		time.Sleep(1 * time.Second)
	}

	serializedRcvMsgCh = make(chan SerializedRcvMsg, 100)
	serializedQueryCh = make(chan SerializedQuery, 100)
	go SerializeDevicesFunc(client)

	for {
		token := client.Subscribe("tasmota/discovery/#", AtLeastOnce,
			func(client mqtt.Client, msg mqtt.Message) {
				m := SerializedRcvMsg{"TasmotaDiscoveryMessageHandler", msg}
				serializedRcvMsgCh <- m
			})
		token.Wait()
		if token.Error() == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	log.Printf("Subscribed to Tasmota Discovery Protocol")

	time.Sleep(5 * time.Second)
	log.Println("MQTT Devices:")
	for _, d := range devices {
		log.Println(d)
	}

}
