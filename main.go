// observerip-proxy-mqtt is a web server obtains data from observerip weather
// station and pushed it to an mqtt server.

package main

import (
	"expvar"
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// Command-line flags.
var (
	httpAddr = flag.String("http", ":8080", "Listen address")
	verbose  = flag.Bool("v", false, "Verbose log")
)

const mph2kph float64 = 1.60934
const projectVersion string = "v0.0.8"
const projectURL string = "https://github.com/glarfs/docker-observerip-proxy-mqtt"

func main() {
	flag.Parse()
	// env variables
	mqttBroker := getEnv("OBSERVER_MQTT_HOST", "192.168.10.1")
	mqttPort := getEnv("OBSERVER_MQTT_PORT", "1883")
	mqttEntryPoint := getEnv("OBSERVER_MQTT_ENTRYPOINT", "/test/meteo")
	mqttClientID := getEnv("OBSERVER_MQTT_CLIENTID", "observerip-proxy")
	proxyURL := getEnv("OBSERVER_PROXY_URL", "http://rtupdate.wunderground.com")
	proxyPath := getEnv("OBSERVER_PROXY_PATH", "/weatherstation/updateweatherstation.php")
	fmt.Printf("configuration:\n verbose: %v\n mqtt:\n  broker: %v\n  port: %v\n  entrypoint: %v\n  clientId: %v\n proxy:\n  url: %v\n  path: %v\n http:\n  port: %v\n", *verbose, mqttBroker, mqttPort, mqttEntryPoint, mqttClientID, proxyURL, proxyPath, *httpAddr)

	//changeURL := fmt.Sprintf("%sgo%s", baseChangeURL, *version)
	http.Handle("/", NewServer(mqttBroker, mqttPort, mqttEntryPoint, mqttClientID, proxyURL, proxyPath, *verbose))
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

type convert func(float64) float64

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
	verbose        bool

	client mqtt.Client
}

// NewServer returns an initialized outyet server.
func NewServer(mqttBroker, mqttPort string, mqttEntryPoint string, mqttClientID string, proxyURL string, proxyPath string, verbose bool) *Server {
	s := &Server{mqttBroker: mqttBroker, mqttPort: mqttPort, mqttEntryPoint: mqttEntryPoint, mqttClientID: mqttClientID, proxyURL: proxyURL, proxyPath: proxyPath, verbose: verbose, client: nil}
	return s
}

func (s *Server) publishParameterConv(entryPoint string, qos byte, retain bool, value string, fn convert) {
	val, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Print(err)
	} else {
		if s.client.IsConnected() {
			mqValue := fmt.Sprintf("%.1f", fn(val))
			if s.verbose {
				log.Printf("entrypoint: %s, calus: %s \n", entryPoint, mqValue)
			}
			s.client.Publish(entryPoint, qos, retain, mqValue)
		}
	}
}
func (s *Server) publishParameter(entryPoint string, qos byte, retain bool, value string) {
	s.publishParameterConv(entryPoint, qos, retain, value, noConvert)
}

// ServeHTTP implements the HTTP user interface.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hitCount.Add(1)
	if s.verbose {
		log.Printf("method: %s, uri: %s, ip: %s, user-agent: %s \n", r.Method, r.RequestURI, r.RemoteAddr, r.Header.Get("User-Agent"))
	}
	if r.Method == http.MethodGet && strings.Contains(r.RequestURI, "updateweatherstation") {

		//?ID=XXXXX5&PASSWORD=******&tempf=51.1&humidity=99&dewptf=50.9&windchillf=51.1&winddir=262&windspeedmph=2.24&windgustmph=4.92&rainin=0.00&dailyrainin=0.00&weeklyrainin=0.00&monthlyrainin=0.00&yearlyrainin=0.00&solarradiation=28.78&UV=1&indoortempf=60.4&indoorhumidity=69&baromin=29.93&lowbatt=0&dateutc=2020-1-22%208:22:34&softwaretype=Weather%20logger%20V2.2.2&action=updateraw&realtime=1&rtfreq=5
		//id := getParameter(r, "ID", true)
		//password := getParameter(r, "PASSWORD", true)

		// connect to mqtt
		if s.client == nil || !s.client.IsConnected() {
			s.client = connect(s.mqttClientID, s.mqttBroker, s.mqttPort)
			if s.verbose {
				log.Printf("connected to mqtt :%s %s %s\n", s.mqttBroker, s.mqttPort, s.mqttEntryPoint)
			}
		}

		if s.client != nil && s.client.IsConnected() {
			log.Printf("send data to mqtt server: %s:%s %s\n ", s.mqttBroker, s.mqttPort, s.mqttEntryPoint)
			//obtain data
			tempf := getParameter(r, "tempf", true)
			humidity := getParameter(r, "humidity", true)
			dewptf := getParameter(r, "dewptf", true)
			windchillf := getParameter(r, "windchillf", true)
			winddir := getParameter(r, "winddir", true)
			windspeedmph := getParameter(r, "windspeedmph", true)
			windgustmph := getParameter(r, "windgustmph", true)
			rainin := getParameter(r, "rainin", true)
			dailyrainin := getParameter(r, "dailyrainin", true)
			weeklyrainin := getParameter(r, "weeklyrainin", true)
			monthlyrainin := getParameter(r, "monthlyrainin", true)
			yearlyrainin := getParameter(r, "yearlyrainin", true)
			//solar
			solarradiation := getParameter(r, "solarradiation", false)
			uv := getParameter(r, "UV", false)
			//internal
			indoortempf := getParameter(r, "indoortempf", true)
			indoorhumidity := getParameter(r, "indoorhumidity", true)
			baromin := getParameter(r, "baromin", true)
			lowbatt := getParameter(r, "lowbatt", true)
			//dateutc := getParameter(r, "dateutc", true)
			softwaretype := getParameter(r, "softwaretype", true)
			//action := getParameter(r, "action", true)
			//realtime := getParameter(r, "realtime", true)
			//rtfreq := getParameter(r, "rtfreq", true)

			// publish the results
			s.client.Publish(fmt.Sprintf("%s/status", s.mqttEntryPoint), 0, true, "1")
			//TODO calculate
			s.publishParameter(fmt.Sprintf("%s/solar/uvi", s.mqttEntryPoint), 0, true, uv)
			s.publishParameter(fmt.Sprintf("%s/solar/uv", s.mqttEntryPoint), 0, true, uv)
			s.publishParameter(fmt.Sprintf("%s/solar/solarradiation", s.mqttEntryPoint), 0, true, solarradiation)
			s.publishParameter(fmt.Sprintf("%s/absPressure", s.mqttEntryPoint), 0, true, baromin)
			//TODO calculate
			s.publishParameter(fmt.Sprintf("%s/relPressure", s.mqttEntryPoint), 0, true, baromin)
			s.publishParameter(fmt.Sprintf("%s/win/dir", s.mqttEntryPoint), 0, true, winddir)
			s.publishParameterConv(fmt.Sprintf("%s/win/speed", s.mqttEntryPoint), 0, true, windspeedmph, mph2kphConvert)
			s.publishParameterConv(fmt.Sprintf("%s/win/gust", s.mqttEntryPoint), 0, true, windgustmph, mph2kphConvert)
			s.publishParameter(fmt.Sprintf("%s/in/humid", s.mqttEntryPoint), 0, true, indoorhumidity)
			s.publishParameter(fmt.Sprintf("%s/out/humid", s.mqttEntryPoint), 0, true, humidity)
			s.publishParameterConv(fmt.Sprintf("%s/in/temp", s.mqttEntryPoint), 0, true, indoortempf, fahrenheit2CelsiusConvert)
			s.publishParameterConv(fmt.Sprintf("%s/out/temp", s.mqttEntryPoint), 0, true, tempf, fahrenheit2CelsiusConvert)
			s.publishParameterConv(fmt.Sprintf("%s/out/dewpoint", s.mqttEntryPoint), 0, true, dewptf, fahrenheit2CelsiusConvert)
			s.publishParameterConv(fmt.Sprintf("%s/out/windchill", s.mqttEntryPoint), 0, true, windchillf, fahrenheit2CelsiusConvert)

			s.publishParameter(fmt.Sprintf("%s/rain/houry", s.mqttEntryPoint), 0, true, rainin)
			s.publishParameter(fmt.Sprintf("%s/rain/daily", s.mqttEntryPoint), 0, true, dailyrainin)
			s.publishParameter(fmt.Sprintf("%s/rain/weekly", s.mqttEntryPoint), 0, true, weeklyrainin)
			s.publishParameter(fmt.Sprintf("%s/rain/monthly", s.mqttEntryPoint), 0, true, monthlyrainin)
			s.publishParameter(fmt.Sprintf("%s/rain/yearly", s.mqttEntryPoint), 0, true, yearlyrainin)
			s.client.Publish(fmt.Sprintf("%s/info", s.mqttEntryPoint), 0, true, softwaretype)
			// WeeWx compatible Mqtt
			s.publishParameterConv(fmt.Sprintf("%s/inTemp_C", s.mqttEntryPoint), 0, true, indoortempf, fahrenheit2CelsiusConvert)
			s.publishParameter(fmt.Sprintf("%s/inHumidity", s.mqttEntryPoint), 0, true, indoorhumidity)
			s.publishParameterConv(fmt.Sprintf("%s/outTemp_C", s.mqttEntryPoint), 0, true, tempf, fahrenheit2CelsiusConvert)
			s.publishParameter(fmt.Sprintf("%s/outHumidity", s.mqttEntryPoint), 0, true, humidity)
			s.publishParameterConv(fmt.Sprintf("%s/dewpoint_C", s.mqttEntryPoint), 0, true, dewptf, fahrenheit2CelsiusConvert)
			s.publishParameterConv(fmt.Sprintf("%s/windchill_C", s.mqttEntryPoint), 0, true, windchillf, fahrenheit2CelsiusConvert)
			s.publishParameterConv(fmt.Sprintf("%s/windSpeed_kph", s.mqttEntryPoint), 0, true, windspeedmph, mph2kphConvert)
			s.publishParameter(fmt.Sprintf("%s/windDir", s.mqttEntryPoint), 0, true, winddir)
			s.publishParameterConv(fmt.Sprintf("%s/windGust_kph", s.mqttEntryPoint), 0, true, windgustmph, mph2kphConvert)
			s.publishParameter(fmt.Sprintf("%s/radiation_Wpm2", s.mqttEntryPoint), 0, true, solarradiation)
			s.publishParameter(fmt.Sprintf("%s/illuminance", s.mqttEntryPoint), 0, true, uv)
			s.publishParameter(fmt.Sprintf("%s/UV", s.mqttEntryPoint), 0, true, uv)
			s.publishParameter(fmt.Sprintf("%s/rainRate_cm_per_hour", s.mqttEntryPoint), 0, true, rainin)
			s.publishParameter(fmt.Sprintf("%s/dayRain_cm", s.mqttEntryPoint), 0, true, dailyrainin)
			s.publishParameter(fmt.Sprintf("%s/pressure_mbar", s.mqttEntryPoint), 0, true, baromin)
			//TODO convert
			s.publishParameter(fmt.Sprintf("%s/altimeter_mbar", s.mqttEntryPoint), 0, true, baromin)
			s.publishParameter(fmt.Sprintf("%s/outBatteryStatus", s.mqttEntryPoint), 0, true, lowbatt)

		} else {
			log.Fatalf("connection failed to mqtt server: %s port: %s\n", s.mqttBroker, s.mqttPort)
		}
		//proxy connection to wunderground
		pBody := "success"
		log.Printf("proxing to url: %s%s\n", s.proxyURL, r.RequestURI)
		pr, err := http.Get(s.proxyURL + r.RequestURI)
		if err != nil {
			log.Fatalf("error proxy connection: %v\n", err)
		} else {
			defer pr.Body.Close()
			body, err := ioutil.ReadAll(pr.Body)
			if err != nil {
				log.Fatalf("error proxy reading: %v\n", err)
			} else {
				pBody = string(body)
			}
		}

		//response
		w.Header().Set("Content-Type", "text/plain; charset=utf-8") // normal header
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(pBody))
	} else {
		data := struct {
			URL         string
			Version     string
			Description string
		}{
			projectURL,
			projectVersion,
			"observerip-proxy-mqtt is a web server obtains data from observerip weather station and pushed it to an mqtt server.",
		}
		log.Printf("web page test: %s\n", r.RequestURI)
		w.Header().Set("Content-Type", "text/html; charset=utf-8") // normal header
		err := tmpl.Execute(w, data)
		if err != nil {
			w.WriteHeader(http.StatusTeapot)
			log.Print(err)
		} else {
			w.WriteHeader(http.StatusOK)
		}

	}
}

// tmpl is the HTML template that drives the user interface.
var tmpl = template.Must(template.New("tmpl").Parse(`
<!DOCTYPE html><html><body><center>
	<h2>observerip-proxy-mqtt {{.Version}} </h2>
	<h1>	
		<a href="{{.URL}}">github</a>	
	</h1>
	<p>{{.Description}}</p>
</center></body></html>
`))

func fahrenheit2CelsiusConvert(f float64) float64 {
	return float64((f - 32) * 5 / 9)
}

func mph2kphConvert(f float64) float64 {
	return float64(f * mph2kph)
}
func noConvert(f float64) float64 {
	return float64(f)
}

func getParameter(r *http.Request, key string, required bool) string {
	value, ok := r.URL.Query()[key]

	if (!ok || len(value[0]) < 1) && required {
		log.Printf("Url required Param '%s' is missing\n", key)
		return ""
	}
	return value[0]
}

// mqtt functions
func connect(clientID string, host string, port string) mqtt.Client {
	opts := createClientOptions(clientID, host, port)
	client := mqtt.NewClient(opts)
	token := client.Connect()
	for !token.WaitTimeout(3 * time.Second) {
	}
	if err := token.Error(); err != nil {
		log.Fatal(err)
		return nil
	}
	return client
}

func createClientOptions(clientID string, host string, port string) *mqtt.ClientOptions {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%s", host, port))
	opts.SetClientID(clientID)
	return opts
}
