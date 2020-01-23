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
	httpAddr   = flag.String("http", ":8080", "Listen address")
	pollPeriod = flag.Duration("poll", 5*time.Second, "Poll period")
	version    = flag.String("version", "1.4", "Go version")
)

const mph2kph float64 = 1.60934

func main() {
	flag.Parse()
	// env variables
	mqttBroker := getEnv("OBSERVER_MQTT_HOST", "192.168.10.1")
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
}

// NewServer returns an initialized outyet server.
func NewServer(mqttBroker, mqttPort string, mqttEntryPoint string, mqttClientID string, proxyURL string, proxyPath string) *Server {
	s := &Server{mqttBroker: mqttBroker, mqttPort: mqttPort, mqttEntryPoint: mqttEntryPoint, mqttClientID: mqttClientID, proxyURL: proxyURL, proxyPath: proxyPath}
	return s
}

// ServeHTTP implements the HTTP user interface.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hitCount.Add(1)
	log.Println("method: %s, uri: %s, ip: %s, user-agent: %s \n", r.Method, r.RequestURI, r.RemoteAddr, r.Header.Get("User-Agent"))
	if r.Method == http.MethodGet && strings.Contains(r.RequestURI, "updateweatherstation") {

		//?ID=XXXXX5&PASSWORD=******&tempf=51.1&humidity=99&dewptf=50.9&windchillf=51.1&winddir=262&windspeedmph=2.24&windgustmph=4.92&rainin=0.00&dailyrainin=0.00&weeklyrainin=0.00&monthlyrainin=0.00&yearlyrainin=0.00&solarradiation=28.78&UV=1&indoortempf=60.4&indoorhumidity=69&baromin=29.93&lowbatt=0&dateutc=2020-1-22%208:22:34&softwaretype=Weather%20logger%20V2.2.2&action=updateraw&realtime=1&rtfreq=5
		//id := getParameter(r, "ID", true)
		//password := getParameter(r, "PASSWORD", true)

		// connect to mqtt
		client := connect(s.mqttClientID, s.mqttBroker, s.mqttPort)
		if client != nil {
			log.Println("send data to mqtt server: %s port: %s", s.mqttBroker, s.mqttPort)
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
			client.Publish(fmt.Sprintf("%s/status", s.mqttEntryPoint), 0, true, "1")
			//TODO calculate
			publishParameter(client, fmt.Sprintf("%s/solar/uvi", s.mqttEntryPoint), 0, false, uv)
			publishParameter(client, fmt.Sprintf("%s/solar/uv", s.mqttEntryPoint), 0, false, uv)
			publishParameter(client, fmt.Sprintf("%s/solar/solarradiation", s.mqttEntryPoint), 0, false, solarradiation)
			publishParameter(client, fmt.Sprintf("%s/absPressure", s.mqttEntryPoint), 0, false, baromin)
			//TODO calculate
			publishParameter(client, fmt.Sprintf("%s/relPressure", s.mqttEntryPoint), 0, false, baromin)
			publishParameter(client, fmt.Sprintf("%s/win/dir", s.mqttEntryPoint), 0, false, winddir)
			publishParameterConv(client, fmt.Sprintf("%s/win/speed", s.mqttEntryPoint), 0, false, windspeedmph, mph2kphConvert)
			publishParameterConv(client, fmt.Sprintf("%s/win/gust", s.mqttEntryPoint), 0, false, windgustmph, mph2kphConvert)
			publishParameter(client, fmt.Sprintf("%s/in/humid", s.mqttEntryPoint), 0, false, indoorhumidity)
			publishParameter(client, fmt.Sprintf("%s/out/humid", s.mqttEntryPoint), 0, false, humidity)
			publishParameterConv(client, fmt.Sprintf("%s/in/temp", s.mqttEntryPoint), 0, false, indoortempf, fahrenheit2CelsiusConvert)
			publishParameterConv(client, fmt.Sprintf("%s/out/temp", s.mqttEntryPoint), 0, false, tempf, fahrenheit2CelsiusConvert)
			publishParameterConv(client, fmt.Sprintf("%s/out/dewpoint", s.mqttEntryPoint), 0, false, dewptf, fahrenheit2CelsiusConvert)
			publishParameterConv(client, fmt.Sprintf("%s/out/windchill", s.mqttEntryPoint), 0, false, windchillf, fahrenheit2CelsiusConvert)

			publishParameter(client, fmt.Sprintf("%s/rain/houry", s.mqttEntryPoint), 0, false, rainin)
			publishParameter(client, fmt.Sprintf("%s/rain/daily", s.mqttEntryPoint), 0, false, dailyrainin)
			publishParameter(client, fmt.Sprintf("%s/rain/weekly", s.mqttEntryPoint), 0, false, weeklyrainin)
			publishParameter(client, fmt.Sprintf("%s/rain/monthly", s.mqttEntryPoint), 0, false, monthlyrainin)
			publishParameter(client, fmt.Sprintf("%s/rain/yearly", s.mqttEntryPoint), 0, false, yearlyrainin)
			publishParameter(client, fmt.Sprintf("%s/info", s.mqttEntryPoint), 0, false, softwaretype)
			// WeeWx compatible Mqtt
			publishParameterConv(client, fmt.Sprintf("%s/inTemp_C", s.mqttEntryPoint), 0, false, indoortempf, fahrenheit2CelsiusConvert)
			publishParameter(client, fmt.Sprintf("%s/inHumidity", s.mqttEntryPoint), 0, false, indoorhumidity)
			publishParameterConv(client, fmt.Sprintf("%s/outTemp_C", s.mqttEntryPoint), 0, false, tempf, fahrenheit2CelsiusConvert)
			publishParameter(client, fmt.Sprintf("%s/outHumidity", s.mqttEntryPoint), 0, false, humidity)
			publishParameterConv(client, fmt.Sprintf("%s/dewpoint_C", s.mqttEntryPoint), 0, false, dewptf, fahrenheit2CelsiusConvert)
			publishParameterConv(client, fmt.Sprintf("%s/windchill_C", s.mqttEntryPoint), 0, false, windchillf, fahrenheit2CelsiusConvert)
			publishParameterConv(client, fmt.Sprintf("%s/windSpeed_kph", s.mqttEntryPoint), 0, false, windspeedmph, mph2kphConvert)
			publishParameter(client, fmt.Sprintf("%s/windDir", s.mqttEntryPoint), 0, false, winddir)
			publishParameterConv(client, fmt.Sprintf("%s/windGust_kph", s.mqttEntryPoint), 0, false, windgustmph, mph2kphConvert)
			publishParameter(client, fmt.Sprintf("%s/radiation_Wpm2", s.mqttEntryPoint), 0, false, solarradiation)
			publishParameter(client, fmt.Sprintf("%s/illuminance", s.mqttEntryPoint), 0, false, uv)
			publishParameter(client, fmt.Sprintf("%s/UV", s.mqttEntryPoint), 0, false, uv)
			publishParameter(client, fmt.Sprintf("%s/rainRate_cm_per_hour", s.mqttEntryPoint), 0, false, rainin)
			publishParameter(client, fmt.Sprintf("%s/dayRain_cm", s.mqttEntryPoint), 0, false, dailyrainin)
			publishParameter(client, fmt.Sprintf("%s/pressure_mbar", s.mqttEntryPoint), 0, false, baromin)
			//TODO convert
			publishParameter(client, fmt.Sprintf("%s/altimeter_mbar", s.mqttEntryPoint), 0, false, baromin)
			publishParameter(client, fmt.Sprintf("%s/outBatteryStatus", s.mqttEntryPoint), 0, false, lowbatt)

		} else {
			log.Println("connection failed to mqtt server: %s port: %s", s.mqttBroker, s.mqttPort)
		}
		//proxy connection to wunderground
		pBody := "success"
		log.Println("proxing to url: %s", s.proxyURL+r.RequestURI)
		pr, err := http.Get(s.proxyURL + r.RequestURI)
		if err != nil {
			log.Println("error proxy connection: %v", err)
		} else {
			defer pr.Body.Close()
			body, err := ioutil.ReadAll(pr.Body)
			if err != nil {
				log.Println("error proxy reading: %v", err)
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
			URL     string
			Version string
			Yes     bool
		}{
			"s.url",
			"s.version",
			true,
		}
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

func fahrenheit2CelsiusConvert(f float64) float64 {
	return float64((f - 32) * 5 / 9)
}

func mph2kphConvert(f float64) float64 {
	return float64(f * mph2kph)
}
func noConvert(f float64) float64 {
	return float64(f)
}

func publishParameterConv(client mqtt.Client, entryPoint string, qos byte, retain bool, value string, fn convert) {
	val, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Print(err)
	} else {
		client.Publish(entryPoint, qos, true, float64(fn(val)))
	}
}
func publishParameter(client mqtt.Client, entryPoint string, qos byte, retain bool, value string) {
	publishParameterConv(client, entryPoint, qos, retain, value, noConvert)
}

func getParameter(r *http.Request, key string, required bool) string {
	keys, ok := r.URL.Query()[key]

	if (!ok || len(keys[0]) < 1) && required {
		log.Println("Url required Param '%s' is missing", key)
		return ""
	}
	return key
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
