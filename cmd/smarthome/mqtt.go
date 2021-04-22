package main

import (
	"encoding/json"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"hash/fnv"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"inet.af/netaddr"
)

const (
	// MQTT QoS values
	AtMostOnce  = 0
	AtLeastOnce = 1
	ExactlyOnce = 2
)

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
	PowerState    string
	OneshotNotify map[string]chan NotifyState
}

// Notification sent to listeners upon receiving a state change from a device.
// The listener transforms this into a Query response or Execute response.
type NotifyState struct {
	Id         string
	PowerState string
}

var client mqtt.Client
var devices = make(map[string]TasmotaDevice)
var deviceLock sync.Mutex
var readyCh chan int
var ProjectId string

func NewDevice() TasmotaDevice {
	var device TasmotaDevice
	device.OneshotNotify = make(map[string]chan NotifyState)
	return device
}

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
	sync.OtherDeviceIds.AgentId = ProjectId
	sync.OtherDeviceIds.DeviceId = device.Hostname

	return sync
}

// Produce the Device portion of a Google Smart Home Query Response
// https://developers.google.com/assistant/smarthome/reference/intent/query
func (device *TasmotaDevice) ToIntentQueryResponseDevice() IntentQueryResponseDevice {
	var query IntentQueryResponseDevice
	query.Id = device.MacAddress
	query.Online = true
	query.Status = "SUCCESS"
	if device.PowerState == "ON" {
		query.On = true
	} else {
		query.On = false
	}

	return query
}

// to be called from fulfillment goroutines to send an MQTT query for the state of a device.
func SendQuery(topic string) {
	retained := false
	token := client.Publish(topic, AtLeastOnce, retained, "QUERY")
	_ = token.Wait()
	if token.Error() != nil {
		log.Printf("DeviceQuery: client.Publish failed: %q\n", token.Error())
	}
}

// to be called from fulfillment goroutines to control the state of the device.
func (device *TasmotaDevice) SendPowerOnOff(On bool) {
	var state string
	if On {
		state = "ON"
	} else {
		state = "OFF"
	}

	topic := "cmnd/" + device.TopicName + "/power"
	retained := false
	token := client.Publish(topic, ExactlyOnce, retained, state)
	go func() {
		_ = token.Wait()
		if token.Error() != nil {
			log.Printf("DeviceExecute: client.Publish failed: %q\n", token.Error())
			return
		}
	}()
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
func parseTasmotaDiscovery(device *TasmotaDevice, jsonStr []byte) error {
	jsonMap := make(map[string]interface{})
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		return err
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
			device.PowerState = item
		}
	}

	device.TopicName = jsonMap["t"].(string)
	return nil
}

// handles /stat/device-topic/RESULT and /tele/device-topic/STATE messages
// serialized through SerializeDevicesFunc
//
// Example (both topics send the same message format):
// {"Time":"2021-03-28T14:46:16","Uptime":"21T16:41:40","UptimeSec":1874500,"Heap":29,
//  "SleepMode":"Dynamic","Sleep":50,"LoadAvg":19,"MqttCount":20,"POWER":"OFF",
//  "Wifi":{"AP":2,"SSId":"MY-SSID","BSSId":"00:11:22:33:44:55","Channel":1,"RSSI":44,
//          "Signal":-78,"LinkCount":17,"Downtime":"0T00:05:18"}}
func parseTasmotaResult(device *TasmotaDevice, jsonStr []byte) error {
	jsonMap := make(map[string]interface{})
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		return err
	}

	device.PowerState = jsonMap["POWER"].(string)

	update := NotifyState{Id: device.MacAddress, PowerState: device.PowerState}
	for key, ch := range device.OneshotNotify {
		ch <- update
		delete(device.OneshotNotify, key)
	}

	return nil
}

func mqttMessageHandler(client mqtt.Client, msg mqtt.Message) {
	t := strings.Split(msg.Topic(), "/")
	deviceLock.Lock()
	defer deviceLock.Unlock()

	if len(t) == 4 && t[0] == "tasmota" && t[1] == "discovery" {
		if t[3] != "config" {
			// we don't process "/tasmota/discovery/+/sensors" yet
			return
		}

		address := t[2]
		device := NewDevice()
		err := parseTasmotaDiscovery(&device, msg.Payload())
		if err != nil {
			log.Println("parseTasmotaDiscovery failed: " + string(msg.Payload()))
			return
		}
		devices[address] = device

		topic := "/cmnd/" + device.TopicName + "/STATE"
		go func() {
			// fetch current state immediately
			SendQuery(topic)
		}()
	} else if len(t) >= 3 && ((t[0] == "stat" && t[2] == "RESULT") || (t[0] == "tele" && t[2] == "STATE")) {
		address := t[1]
		device, ok := devices[address]
		if ok {
			err := parseTasmotaResult(&device, msg.Payload())
			if err != nil {
				log.Println("parseTasmotaResult failed: " + string(msg.Payload()))
				return
			}
			devices[address] = device
		} else {
			// a device we are ignoring
		}
	} else if len(t) == 3 && t[0] == "tmp" && t[2] == "READY" {
		// This is our own message, sent during init and intended as a signal
		// that we've received all retained messages on other topics.
		readyCh <- 1
	}
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

func HashString(s string) string {
	h := fnv.New64()
	h.Write([]byte(s))
	return strconv.FormatUint(h.Sum64(), 36)
}

// Make one attempt to connect to the MQTT broker. Expected to be called from a loop.
func ConnectToMQTT(slug string) (client mqtt.Client, err error) {
	opts := mqtt.NewClientOptions()

	addr := os.Getenv("MQTT_IP_ADDR")
	ip, err := netaddr.ParseIP(addr)
	if err == nil && ip.Is6() {
		addr = "[" + addr + "]"
	}
	broker := "mqtt://" + addr + ":" + os.Getenv("MQTT_PORT")
	opts.AddBroker(broker)
	opts.SetUsername(os.Getenv("MQTT_USERNAME"))
	opts.SetPassword(os.Getenv("MQTT_PASSWORD"))

	// improve average latency by allowing packets to arrive out of order.
	// everything we do is stateless, operations cannot depend on previous ops.
	opts.SetOrderMatters(false)

	// Only one client with the same ID can connect, add a random slug at end
	opts.SetClientID("CloudRun:" + slug[:12])

	// Cloud Run spins up an instance when an HTTP request arrives, the longer it
	// takes to respond the longer the latency to the user. We want to start trying
	// to connect to the tailscaled SOCKS5 proxy before tailscaled has managed to
	// connect to the Tailnet, so the first few attempts will definitely fail.
	// There is no downside to retrying frequently, until Tailscale connects.
	opts.SetConnectRetry(true)
	opts.SetConnectRetryInterval(500 * time.Millisecond)
	opts.SetAutoReconnect(true)
	opts.SetMaxReconnectInterval(500 * time.Millisecond)
	opts.SetWriteTimeout(30 * time.Second)
	opts.SetPingTimeout(30 * time.Second)

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

func GetMetadata(urlPath string) string {
	client := &http.Client{}
	url := "http://metadata.google.internal/computeMetadata/" + urlPath
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Unable to allocate http.NewRequest: %q\n", err)
		return ""
	}
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("HTTP GET failed: %q\n", err)
		return ""
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("HTTP GET ReadAll failed: %q\n", err)
		return ""
	}

	return string(body)
}

func MQTT() {
	ProjectId = GetMetadata("v1/project/project-id")
	InstanceId := GetMetadata("v1/instance/id")
	slug := HashString(InstanceId)
	readyCh = make(chan int, 1)
	mqtt.ERROR = log.New(os.Stdout, "[MQTT ERROR] ", 0)
	mqtt.CRITICAL = log.New(os.Stdout, "[MQTT CRIT] ", 0)
	mqtt.WARN = log.New(os.Stdout, "[MQTT WARN]  ", 0)
	//mqtt.DEBUG = log.New(os.Stdout, "[MQTT DEBUG] ", 0) // quite verbose

	var err error
	for client, err = ConnectToMQTT(slug); err != nil; {
		time.Sleep(1 * time.Second)
	}

	readyTopic := "tmp/" + slug + "/READY"
	for {
		topics := map[string]byte{
			"tasmota/discovery/#": AtLeastOnce,
			"stat/+/RESULT":       AtLeastOnce,
			"tele/+/STATE":        AtLeastOnce,
			readyTopic:            AtLeastOnce,
		}
		token := client.SubscribeMultiple(topics, mqttMessageHandler)
		token.Wait()
		if token.Error() == nil {
			break
		}
		time.Sleep(1 * time.Second)
	}
	log.Println("Subscribed to MQTT Topics")

	// Send a sentinal to infer whether we've received all retained discovery messages.
	retained := false
	token := client.Publish(readyTopic, AtLeastOnce, retained, "DISCOVERY")
	_ = token.Wait()
	if token.Error() != nil {
		log.Panicf("Publish READY failed: %q\n", token.Error())
	}
	<-readyCh
	log.Printf("Discovered %d MQTT devices\n", len(devices))

	// Send another sentinal to infer whether we've received all state queries
	retained = false
	token = client.Publish(readyTopic, AtLeastOnce, retained, "QUERY")
	_ = token.Wait()
	if token.Error() != nil {
		log.Panicf("Publish READY failed: %q\n", token.Error())
	}
	<-readyCh

	log.Println("Completed MQTT Initialization")
}
