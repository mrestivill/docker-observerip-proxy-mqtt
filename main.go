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
const projectVersion string = "v1.0.0"
const projectURL string = "https://github.com/glarfs/docker-observerip-proxy-mqtt"
const errorValue float64 = -100

func main() {
	flag.Parse()
	// env variables
	mqttBroker := getEnv("OBSERVER_MQTT_HOST", "192.168.10.1")
	mqttPort := getEnv("OBSERVER_MQTT_PORT", "1883")
	mqttEntryPoint := getEnv("OBSERVER_MQTT_ENTRYPOINT", "/test/meteo")
	mqttClientID := getEnv("OBSERVER_MQTT_CLIENTID", "observerip-proxy")
	proxy := getEnv("OBSERVER_PROXY_ENABLED", "true")
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

	client            mqtt.Client
	oldTempf          string
	oldHumidity       string
	oldDewptf         string
	oldWindchillf     string
	oldWinddir        string
	oldWindspeedmph   string
	oldWindgustmph    string
	oldRainin         string
	oldDailyrainin    string
	oldWeeklyrainin   string
	oldMonthlyrainin  string
	oldYearlyrainin   string
	oldSolarradiation string
	oldUv             string
	oldIndoortempf    string
	oldIndoorhumidity string
	oldBaromin        string
	oldLowbatt        string

	//weewx
	oldWeewxIndoortempf    string
	oldWeewxIndoorhumidity string
	oldWeewxTempf          string
	oldWeewxHumidity       string
	oldWeewxDewptf         string
	oldWeewxWindchillf     string
	oldWeewxWindspeedmph   string
	oldWeewxWinddir        string
	oldWeewxWindgustmph    string
	oldWeewxSolarradiation string
	oldWeewxUv             string
	oldWeewxRainin         string
	oldWeewxDailyrainin    string
	oldWeewxBaromin        string
	oldWeewxLowbatt        string
}

// NewServer returns an initialized outyet server.
func NewServer(mqttBroker, mqttPort string, mqttEntryPoint string, mqttClientID string, proxyURL string, proxyPath string, verbose bool) *Server {
	s := &Server{mqttBroker: mqttBroker, mqttPort: mqttPort, mqttEntryPoint: mqttEntryPoint, mqttClientID: mqttClientID, proxyURL: proxyURL, proxyPath: proxyPath, verbose: verbose, client: nil}
	return s
}

func (s *Server) publishParameterConv(entryPoint string, qos byte, retain bool, value string, oldValue string, fn convert) string {
	val, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Print(err)
	} else {
		if s.client.IsConnected() && val > errorValue {
			mqValue := fmt.Sprintf("%.1f", fn(val))
			if s.verbose {
				log.Printf("entrypoint: %s, calus: %s \n", entryPoint, mqValue)
			}
			if mqValue != oldValue {
				s.client.Publish(entryPoint, qos, retain, mqValue)
			}
			return mqValue
		}
	}
	return ""
}
func (s *Server) publishParameter(entryPoint string, qos byte, retain bool, value string, oldValue string) string {
	return s.publishParameterConv(entryPoint, qos, retain, value, oldValue, noConvert)
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
			_ = s.publishParameter(fmt.Sprintf("%s/solar/uvi", s.mqttEntryPoint), 0, false, uv, s.oldUv)
			s.oldUv = s.publishParameter(fmt.Sprintf("%s/solar/uv", s.mqttEntryPoint), 0, false, uv, s.oldUv)
			s.oldSolarradiation = s.publishParameter(fmt.Sprintf("%s/solar/radiation", s.mqttEntryPoint), 0, false, solarradiation, s.oldSolarradiation)
			s.oldBaromin = s.publishParameter(fmt.Sprintf("%s/absPressure", s.mqttEntryPoint), 0, false, baromin, s.oldBaromin)
			//TODO calculate
			s.oldBaromin = s.publishParameter(fmt.Sprintf("%s/relPressure", s.mqttEntryPoint), 0, false, baromin, s.oldBaromin)
			s.oldWinddir = s.publishParameter(fmt.Sprintf("%s/wind/dir", s.mqttEntryPoint), 0, false, winddir, s.oldWinddir)
			s.oldWindspeedmph = s.publishParameterConv(fmt.Sprintf("%s/wind/speed", s.mqttEntryPoint), 0, false, windspeedmph, s.oldWindspeedmph, mph2kphConvert)
			s.oldWindgustmph = s.publishParameterConv(fmt.Sprintf("%s/wind/gust", s.mqttEntryPoint), 0, false, windgustmph, s.oldWindgustmph, mph2kphConvert)
			_ = s.publishParameter(fmt.Sprintf("%s/in/humid", s.mqttEntryPoint), 0, false, indoorhumidity, s.oldIndoorhumidity)
			s.oldIndoorhumidity = s.publishParameter(fmt.Sprintf("%s/out/humid", s.mqttEntryPoint), 0, false, humidity, s.oldIndoorhumidity)
			_ = s.publishParameterConv(fmt.Sprintf("%s/in/temp", s.mqttEntryPoint), 0, false, indoortempf, s.oldIndoortempf, fahrenheit2CelsiusConvert)
			s.oldIndoortempf = s.publishParameterConv(fmt.Sprintf("%s/out/temp", s.mqttEntryPoint), 0, false, tempf, s.oldIndoortempf, fahrenheit2CelsiusConvert)
			s.oldDewptf = s.publishParameterConv(fmt.Sprintf("%s/out/dewpoint", s.mqttEntryPoint), 0, false, dewptf, s.oldDewptf, fahrenheit2CelsiusConvert)
			s.oldWindchillf = s.publishParameterConv(fmt.Sprintf("%s/out/windchill", s.mqttEntryPoint), 0, false, windchillf, s.oldWindchillf, fahrenheit2CelsiusConvert)

			s.oldRainin = s.publishParameter(fmt.Sprintf("%s/rain/houry", s.mqttEntryPoint), 0, false, rainin, s.oldRainin)
			s.oldDailyrainin = s.publishParameter(fmt.Sprintf("%s/rain/daily", s.mqttEntryPoint), 0, false, dailyrainin, s.oldDailyrainin)
			s.oldWeeklyrainin = s.publishParameter(fmt.Sprintf("%s/rain/weekly", s.mqttEntryPoint), 0, false, weeklyrainin, s.oldWeeklyrainin)
			s.oldMonthlyrainin = s.publishParameter(fmt.Sprintf("%s/rain/monthly", s.mqttEntryPoint), 0, false, monthlyrainin, s.oldMonthlyrainin)
			s.oldYearlyrainin = s.publishParameter(fmt.Sprintf("%s/rain/yearly", s.mqttEntryPoint), 0, false, yearlyrainin, s.oldYearlyrainin)
			s.oldLowbatt = s.publishParameter(fmt.Sprintf("%s/out/battery", s.mqttEntryPoint), 0, false, lowbatt, s.oldLowbatt)
			s.client.Publish(fmt.Sprintf("%s/info", s.mqttEntryPoint), 0, false, softwaretype)
			// WeeWx compatible Mqtt
			s.oldWeewxIndoortempf = s.publishParameterConv(fmt.Sprintf("%s/inTemp_C", s.mqttEntryPoint), 0, false, indoortempf, s.oldWeewxIndoortempf, fahrenheit2CelsiusConvert)
			s.oldWeewxIndoorhumidity = s.publishParameter(fmt.Sprintf("%s/inHumidity", s.mqttEntryPoint), 0, false, indoorhumidity, s.oldWeewxIndoorhumidity)
			s.oldWeewxTempf = s.publishParameterConv(fmt.Sprintf("%s/outTemp_C", s.mqttEntryPoint), 0, false, tempf, s.oldWeewxTempf, fahrenheit2CelsiusConvert)
			s.oldWeewxHumidity = s.publishParameter(fmt.Sprintf("%s/outHumidity", s.mqttEntryPoint), 0, false, humidity, s.oldWeewxHumidity)
			s.oldWeewxDewptf = s.publishParameterConv(fmt.Sprintf("%s/dewpoint_C", s.mqttEntryPoint), 0, false, dewptf, s.oldWeewxDewptf, fahrenheit2CelsiusConvert)
			s.oldWeewxWindchillf = s.publishParameterConv(fmt.Sprintf("%s/windchill_C", s.mqttEntryPoint), 0, false, windchillf, s.oldWeewxWindchillf, fahrenheit2CelsiusConvert)
			s.oldWeewxWindspeedmph = s.publishParameterConv(fmt.Sprintf("%s/windSpeed_kph", s.mqttEntryPoint), 0, false, windspeedmph, s.oldWeewxWindspeedmph, mph2kphConvert)
			s.oldWeewxWinddir = s.publishParameter(fmt.Sprintf("%s/windDir", s.mqttEntryPoint), 0, false, winddir, s.oldWeewxWinddir)
			s.oldWeewxWindgustmph = s.publishParameterConv(fmt.Sprintf("%s/windGust_kph", s.mqttEntryPoint), 0, false, windgustmph, s.oldWeewxWindgustmph, mph2kphConvert)
			s.oldWeewxSolarradiation = s.publishParameter(fmt.Sprintf("%s/radiation_Wpm2", s.mqttEntryPoint), 0, false, solarradiation, s.oldWeewxSolarradiation)
			_ = s.publishParameter(fmt.Sprintf("%s/illuminance", s.mqttEntryPoint), 0, false, uv, s.oldWeewxUv)
			s.oldWeewxUv = s.publishParameter(fmt.Sprintf("%s/UV", s.mqttEntryPoint), 0, false, uv, s.oldWeewxUv)
			s.oldWeewxRainin = s.publishParameter(fmt.Sprintf("%s/rainRate_cm_per_hour", s.mqttEntryPoint), 0, false, rainin, s.oldWeewxRainin)
			s.oldWeewxDailyrainin = s.publishParameter(fmt.Sprintf("%s/dayRain_cm", s.mqttEntryPoint), 0, false, dailyrainin, s.oldWeewxDailyrainin)
			_ = s.publishParameter(fmt.Sprintf("%s/pressure_mbar", s.mqttEntryPoint), 0, false, baromin, s.oldWeewxBaromin)
			//TODO convert
			s.oldWeewxBaromin = s.publishParameter(fmt.Sprintf("%s/altimeter_mbar", s.mqttEntryPoint), 0, false, baromin, s.oldWeewxBaromin)
			s.oldWeewxLowbatt = s.publishParameter(fmt.Sprintf("%s/outBatteryStatus", s.mqttEntryPoint), 0, false, lowbatt, s.oldWeewxLowbatt)

		} else {
			log.Fatalf("connection failed to mqtt server: %s port: %s\n", s.mqttBroker, s.mqttPort)
		}
		pBody := "success"
		//proxy connection to wunderground
		if val, err := strconv.ParseBool(s.proxy); err == nil {
			if val ==true {			
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
			}
		} else {
        		log.Fatalf("Given OBSERVER_PROXY_ENABLED is not a bool")
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
