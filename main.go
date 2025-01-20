package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

type Sensor struct {
	Name  string
	State struct {
		Temperature int
		Status      int
		LastUpdated string
		Presence    bool
		LightLevel  int
	}
}

type SensorReport struct {
	Reporter string            `json:"reporter"`
	Topic    string            `json:"topic"`
	Sensors  map[string]string `json:"sensors"`
}

var httpClient *http.Client = &http.Client{
	Transport: &http.Transport{
		DisableKeepAlives: true,
	},
}

func initMetrics() {
	go func() {
		if config.PhilipsHueUrl == "" {
			logrus.Info("Philips Hue URL not set")
			return
		}

		for {
			response, err := http.Get(config.PhilipsHueUrl + "/sensors")
			if err != nil {
				logrus.Errorf("Failed to get sensor data: %v", err)
			} else {
				bodyBytes, err := io.ReadAll(response.Body)
				if err != nil {
					logrus.Errorf("Failed to read sensor response: %v", err)
				}

				var sensorResponse = make(map[string]Sensor)
				err = json.Unmarshal(bodyBytes, &sensorResponse)
				if err != nil {
					logrus.Errorf("Unmarshal of sensor response failed: %v", err)
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
		retry := true
		var sensorReport = SensorReport{
			Reporter: "hue-sensor-agent",
			Topic:    "sensors",
			Sensors: map[string]string{
				"hue_temperature": fmt.Sprintf("%.2f", float32(temperatureSensor.State.Temperature)/100.0),
				"hue_presence":    strconv.FormatBool(motionSensor.State.Presence),
				"hue_lightlevel":  fmt.Sprintf("%d", lightLevelSensor.State.LightLevel),
			},
		}

		logrus.Debugf("Sending report to %v", config.SensorRelayUrl)

		reportJson, _ := json.Marshal(sensorReport)
	retry:
		response, err := httpClient.Post(config.SensorRelayUrl, "application/json", bytes.NewBuffer(reportJson))
		if err != nil {
			if err == io.EOF && retry {
				logrus.Info("Got EOF, retrying")
				goto retry
			}
			logrus.Error("Failed to send report: " + err.Error())
		} else {
			defer response.Body.Close()
		}

	} else {
		logrus.Infof("No sensor report URL configured")
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

	strEnvMap := map[string]*string{
		"PHILIPS_HUE_URL":       &config.PhilipsHueUrl,
		"MOTION_SENSOR_ID":      &config.MotionSensorId,
		"LIGHT_LEVEL_SENSOR_ID": &config.LightLevelSensorId,
		"TEMPERATURE_SENSOR_ID": &config.TemperatureSensorId,
		"SENSOR_RELAY_URL":      &config.SensorRelayUrl,
	}

	for key, configRef := range strEnvMap {
		value, exist := os.LookupEnv(key)
		if exist {
			*configRef = value
		}
	}

	intEnvMap := map[string]*int{
		"PORT": &config.Port,
	}

	for key, configRef := range intEnvMap {
		value, exist := os.LookupEnv(key)
		if exist {
			*configRef, _ = strconv.Atoi(value)
		}
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
	PhilipsHueUrl       string
	MotionSensorId      string
	LightLevelSensorId  string
	TemperatureSensorId string
	Port                int
	SensorRelayUrl      string
}

var config Config

func main() {

	configPath := flag.String("c", "config.json", "Path to configuration file")
	flag.Parse()

	initConfig(*configPath)
	initMetrics()

	http.Handle("/metrics", promhttp.Handler())
	logrus.Infof("Listening to :%d\n", config.Port)
	log.Fatal("%+v\n", http.ListenAndServe(fmt.Sprintf(":%d", config.Port), nil))
}
