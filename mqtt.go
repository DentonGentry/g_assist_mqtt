package main

import (
	"encoding/json"
	"fmt"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"os"
	"strings"
	"time"
)

type TasmotaDevice struct {
	IP           string
	FriendlyName string
	Hostname     string
}

var devices = make(map[string]TasmotaDevice)

// Parse JSON received on tasmota/discovery/*/config
// {"ip":"10.1.10.1","dn":"Tasmota",
//  "fn":["ParentsRoomSwitch",null,null,null,null,null,null,null],
//  "hn":"parents-room-switch","mac":"BCDDC2000000","md":"MJ-S01 Switch","ty":0,"if":0,
//  "ofln":"Offline","onln":"Online","state":["OFF","ON","TOGGLE","HOLD"],"sw":"9.3.1",
//  "t":"parents-room-switch","ft":"%prefix%/%topic%/","tp":["cmnd","stat","tele"],
//  "rl":[1,0,0,0,0,0,0,0],"swc":[-1,-1,-1,-1,-1,-1,-1,-1],
//  "swn":[null,null,null,null,null,null,null,null],"btn":[0,0,0,0,0,0,0,0],
//  "so":{"4":0,"11":0,"13":0,"17":0,"20":0,"30":0,"68":0,"73":0,"82":0,"114":0,"117":0},
//  "lk":1,"lt_st":0,"sho":[0,0,0,0],"ver":1}
func ParseDeviceDiscovery(jsonStr []byte) (TasmotaDevice, error) {
	var device TasmotaDevice

	jsonMap := make(map[string]interface{})
	err := json.Unmarshal(jsonStr, &jsonMap)
	if err != nil {
		return device, err
	}

	device.IP = jsonMap["ip"].(string)
	fn := jsonMap["fn"].([]interface{})
	device.FriendlyName = fn[0].(string)
	device.Hostname = jsonMap["hn"].(string)

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

func subscribeDiscover(client mqtt.Client) {
	topic := "tasmota/discovery/#"
	token := client.Subscribe(topic, 1, nil)
	token.Wait()
	fmt.Printf("Subscribed to topic: %s\n", topic)
}

func setupMqtt() (mqtt.Client, error) {
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
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return client, token.Error()
	}

	return client, nil
}

func main() {
	client, err := setupMqtt()
	if err != nil {
		panic(err)
	}
	subscribeDiscover(client)
	time.Sleep(60 * time.Second)
	fmt.Println(devices)
}
