package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
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

func initMetrics() {
	go func() {
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
			}

			time.Sleep(10 * time.Second)
		}
	}()
}

func initConfig(configPath string) {

	file, err := os.Open(configPath)
	if err != nil {
		panic(err)
	}

	defer file.Close()

	decoder := json.NewDecoder(file)
	config = Config{}
	err = decoder.Decode(&config)
	if err != nil {
		panic(err)
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
}

var config Config

func main() {

	configPath := flag.String("c", "config.json", "Path to configuration file")
	flag.Parse()

	initConfig(*configPath)
	initMetrics()

	http.Handle("/metrics", promhttp.Handler())
	fmt.Printf("Listening to :%d", config.Port)
	log.Fatal("%+v\n", http.ListenAndServe(fmt.Sprintf(":%d", config.Port), nil));
}
