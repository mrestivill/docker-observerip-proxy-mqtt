/*
Copyright 2014 Google Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// outyet is a web server that announces whether or not a particular Go version
// has been tagged.
package main

import (
	"expvar"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Command-line flags.
var (
	httpAddr   = flag.String("http", ":8080", "Listen address")
	pollPeriod = flag.Duration("poll", 5*time.Second, "Poll period")
	version    = flag.String("version", "1.4", "Go version")
)

const baseChangeURL = "https://go.googlesource.com/go/+/"

func main() {
	flag.Parse()
	// env variables
	mqttBroker := getEnv("OBSERVER_MQTT_HOST", "192.168.1.1")
	mqttPort := getEnv("OBSERVER_MQTT_PORT", "1883")
	mqttEntryPoint := getEnv("OBSERVER_MQTT_ENTRYPOINT", "/test/meteo")
	mqttClientID := getEnv("OBSERVER_MQTT_CLIENTID", "observerip-proxy")
	proxyURL := getEnv("OBSERVER_PROXY_URL", "http://rtupdate.wunderground.com")
	proxyPath := getEnv("OBSERVER_PROXY_PATH", "/weatherstation/updateweatherstation.php")
	fmt.Printf("configuration:\n mqtt:\n  broker: %v\n  port: %v\n  entrypoint: %v\n  clientId: %v\n proxy:\n  url: %v\n  path: %v\n http:\n  port: %v\n", mqttBroker, mqttPort, mqttEntryPoint, mqttClientID, proxyURL, proxyPath, *httpAddr)

	//changeURL := fmt.Sprintf("%sgo%s", baseChangeURL, *version)
	http.Handle("/", NewServer(mqttBroker, mqttPort, mqttEntryPoint, mqttClientID, proxyURL, proxyPath))
	log.Fatal(http.ListenAndServe(*httpAddr, nil))
}
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// Exported variables for monitoring the server.
// These are exported via HTTP as a JSON object at /debug/vars.
var (
	hitCount       = expvar.NewInt("hitCount")
	pollCount      = expvar.NewInt("pollCount")
	pollError      = expvar.NewString("pollError")
	pollErrorCount = expvar.NewInt("pollErrorCount")
)

// Server implements the outyet server.
// It serves the user interface (it's an http.Handler)
// and polls the remote repository for changes.
type Server struct {
	mqttBroker     string
	mqttPort       string
	mqttEntryPoint string
	mqttClientID   string
	proxyURL       string
	proxyPath      string
}

// NewServer returns an initialized outyet server.
func NewServer(mqttBroker, mqttPort string, mqttEntryPoint string, mqttClientID string, proxyURL string, proxyPath string) *Server {
	s := &Server{mqttBroker: mqttBroker, mqttPort: mqttPort, mqttEntryPoint: mqttEntryPoint, mqttClientID: mqttClientID, proxyURL: proxyURL, proxyPath: proxyPath}
	return s
}

// ServeHTTP implements the HTTP user interface.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hitCount.Add(1)
	if r.Method == http.MethodPost {
		client := connect(s.mqttClientID, s.mqttBroker, s.mqttPort)
		client.Publish(fmt.Sprintf("%s/status", s.mqttEntryPoint), 0, true, "1")
	} else {
		data := struct {
			URL     string
			Version string
			Yes     bool
		}{
			"s.url",
			"s.version",
			true,
		}
		err := tmpl.Execute(w, data)
		if err != nil {
			log.Print(err)
		}
	}
}

// tmpl is the HTML template that drives the user interface.
var tmpl = template.Must(template.New("tmpl").Parse(`
<!DOCTYPE html><html><body><center>
	<h2>Is Go {{.Version}} out yet?</h2>
	<h1>
	{{if .Yes}}
		<a href="{{.URL}}">YES!</a>
	{{else}}
		No. :-(
	{{end}}
	</h1>
</center></body></html>
`))

// mqtt functions
func connect(clientID string, host string, port string) mqtt.Client {
	opts := createClientOptions(clientID, host, port)
	client := mqtt.NewClient(opts)
	token := client.Connect()
	for !token.WaitTimeout(3 * time.Second) {
	}
	if err := token.Error(); err != nil {
		log.Fatal(err)
	}
	return client
}

func createClientOptions(clientID string, host string, port string) *mqtt.ClientOptions {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s", host))
	opts.SetClientID(clientID)
	return opts
}
