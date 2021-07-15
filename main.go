package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"time"

	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Sensor struct {
	Name string
	State struct {
		Temperature int
		Status int
		LastUpdated string
		Presence bool
		LightLevel int
	}
}

type SensorReport struct {
	Reporter string `json:"reporter"`
	Topic string `json:"topic"`
	Sensors map[string]string `json:"sensors"`
}

func initMetrics() {
	go func() {
		if config.PhilipsHueUrl == "" {
			return
		}

		for {
			response, err := http.Get(config.PhilipsHueUrl + "/sensors")
			if err != nil {
				fmt.Println(err)
			} else {
				bodyBytes, err := ioutil.ReadAll(response.Body)
				if err != nil {
					fmt.Printf("%+v\n", err)
				}

				var sensorResponse = make(map[string]Sensor)
				err = json.Unmarshal(bodyBytes, &sensorResponse)
				if err != nil {
					fmt.Println(err)
				}

				motionSensor := sensorResponse[config.MotionSensorId]
				presence := 0.0
				if motionSensor.State.Presence {
					presence = 1.0
				}
				presenceGauge.Set(presence)

				temperatureSensor := sensorResponse[config.TemperatureSensorId]
				temperatureGague.Set(float64(temperatureSensor.State.Temperature))

				lightLevelSensor := sensorResponse[config.LightLevelSensorId]
				lightLevelGauge.Set(float64(lightLevelSensor.State.LightLevel))

				sendSensorReport(temperatureSensor, motionSensor, lightLevelSensor)
			}

			time.Sleep(10 * time.Second)
		}
	}()
}

func sendSensorReport(temperatureSensor Sensor, motionSensor Sensor, lightLevelSensor Sensor) {

	if config.SensorRelayUrl != "" {
		var sensorReport = SensorReport{
			Reporter: "hue-sensor-agent",
			Topic:    "sensors",
			Sensors: map[string]string{
				"hue_temperature": fmt.Sprintf("%.2f", float32(temperatureSensor.State.Temperature)/100.0),
				"hue_presence":    strconv.FormatBool(motionSensor.State.Presence),
				"hue_lightlevel":  fmt.Sprintf("%d", lightLevelSensor.State.LightLevel),
			},
		}
		reportJson, _ := json.Marshal(sensorReport)
		response, err := http.Post(config.SensorRelayUrl, "application/json", bytes.NewBuffer(reportJson))
		if err != nil {
			println("Failed to send report: " + response.Status)
		}
	}
}

func initConfig(configPath string) {

	config = Config{
		Port: 9101,
	}

	if _, err := os.Stat(configPath); err == nil {
		file, err := os.Open(configPath)
		if err != nil {
			panic(err)
		}

		defer file.Close()

		decoder := json.NewDecoder(file)
		err = decoder.Decode(&config)
		if err != nil {
			panic(err)
		}

	} else if !os.IsNotExist(err) {
		panic(err)
	}

	strEnvMap := map[string]*string {
		"PHILIPS_HUE_URL": &config.PhilipsHueUrl,
		"MOTION_SENSOR_ID": &config.MotionSensorId,
		"LIGHT_LEVEL_SENSOR_ID": &config.LightLevelSensorId,
		"TEMPERATURE_SENSOR_ID": &config.TemperatureSensorId,
		"SENSOR_RELAY_URL": &config.SensorRelayUrl,
	}

	for key, configRef := range strEnvMap {
		value, exist := os.LookupEnv(key)
		if exist { *configRef = value }
	}

	intEnvMap := map[string]*int {
		"PORT": &config.Port,
	}

	for key, configRef := range intEnvMap {
		value, exist := os.LookupEnv(key)
		if exist { *configRef, _ = strconv.Atoi(value) }
	}
}

var (
	temperatureGague = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "sensor_temperature",
		Help: "Temperature",
	})
	presenceGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "sensor_presence",
		Help: "Motion Detector",
	})
	lightLevelGauge = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "sensor_light_level",
		Help: "Light Level",
	})
)

type Config struct {
	PhilipsHueUrl string
	MotionSensorId string
	LightLevelSensorId string
	TemperatureSensorId string
	Port int
	SensorRelayUrl string
}

var config Config

func main() {

	configPath := flag.String("c", "config.json", "Path to configuration file")
	flag.Parse()

	initConfig(*configPath)
	initMetrics()

	http.Handle("/metrics", promhttp.Handler())
	fmt.Printf("Listening to :%d\n", config.Port)
	log.Fatal("%+v\n", http.ListenAndServe(fmt.Sprintf(":%d", config.Port), nil));
}
