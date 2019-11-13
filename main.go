package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-contrib/sse"
	"github.com/gin-gonic/gin"
	"github.com/robinson/gos7"
)

var plcAddress string

func base64Encode(str string) string {
	return base64.StdEncoding.EncodeToString([]byte(str))
}

func base64Decode(str string) (string, bool) {
	data, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		return "", true
	}
	return string(data), false
}

// DataRecord - Rekord danych
// ========================================================
type DataRecord struct {
	Timestamp int64  `gorm:"AUTO_INCREMENT" form:"v" json:"Timestamp"`
	IOImage   []byte `gorm:"not null" form:"IOImage" json:"IOImage"`
}

// machineTimeline - Dane
// ========================================================
var machineTimeline []DataRecord

// ErrCheck - obsługa błedów
// ================================================================================================
func ErrCheck(errNr error) bool {
	if errNr != nil {
		fmt.Println(errNr)
		return false
	}
	return true
}

// Options - Obsługa request'u OPTIONS (CORS)
// ================================================================================================
func Options(c *gin.Context) {
	if c.Request.Method != "OPTIONS" {
		c.Next()
	} else {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "authorization, origin, content-type, accept")
		c.Header("Allow", "HEAD,GET,POST,PUT,PATCH,DELETE,OPTIONS")
		c.Header("Content-Type", "application/json")
		// c.AbortWithStatus(http.StatusOK)
	}
}

//
// SendData - Wysłanie całej tablicy
// ================================================================================================
func SendData(c *gin.Context) {
	// Typ połączania
	c.Header("Access-Control-Allow-Origin", "*")
	log.Println("GetData()")

	// log.Println(machineTimeline[0].Timestamp)

	data, _ := json.Marshal(machineTimeline)
	// log.Println(string(data))

	c.JSON(http.StatusOK, string(data))
}

//
// eventHandler - Zdarzenia
// ================================================================================================
func eventHandler(c *gin.Context) {
	// func eventHandler(w http.ResponseWriter, req *http.Request) {

	plcAddress := c.Query("plc_address")
	slotNr, _ := strconv.Atoi(c.Query("slot_nr"))
	period, _ := strconv.Atoi(c.Query("period"))

	if net.ParseIP(plcAddress) != nil {
		log.Println("Odbebrałem adres IP: " + plcAddress)

		// TCPClient
		handler := gos7.NewTCPClientHandler(plcAddress, 0, slotNr)
		handler.Timeout = time.Duration(period*1000000) * time.Millisecond
		handler.IdleTimeout = 5 * time.Second
		handler.PDULength = 960
		// handler.Logger = log.New(os.Stdout, plcAddress+" : ", log.LstdFlags)

		// Connect manually so that multiple requests are handled in one connection session
		handler.Connect()
		defer handler.Close()

		client := gos7.NewClient(handler)
		bufEB := make([]byte, 128)
		bufMB := make([]byte, 128)

		// Typ połączania
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Content-Type", "text/event-stream")
		c.Header("Connection", "Keep-Alive")
		c.Header("Transfer-Encoding", "chunked")
		c.Header("X-Accel-Buffering", "no")
		c.Header("Cache-Control", "no-cache")

		log.Println("eventHandler()")
		c.JSON(http.StatusOK, "eventHandler")

		w := c.Writer

		clientGone := w.CloseNotify()
		var closed bool

		go func() {
			<-clientGone
			closed = true
		}()

		log.Println("LOOP start for PLC IP " + plcAddress + " ...")

		var ix int
		lastTime := time.Now().UnixNano()

		for {

			// Jeżeli połączenie zamknięte to break
			if closed {
				log.Println("Client Gone...")
				break
			}

			readTimeStart := time.Now().UnixNano()
			client.AGReadEB(0, 128, bufEB)
			client.AGReadMB(0, 128, bufMB)
			readTimeEnd := time.Now().UnixNano()

			var buf []byte
			for index := range bufMB {
				buf = append(buf, bufEB[index])
			}
			for index := range bufMB {
				buf = append(buf, bufMB[index])
			}

			dane := map[string]interface{}{
				"time":    readTimeEnd,
				"content": buf,
			}

			machineTimeline = append(machineTimeline, DataRecord{Timestamp: readTimeEnd, IOImage: buf})

			// Wysyłamy do VISU co 500 ms

			if readTimeEnd-lastTime > 500000000 {

				sse.Encode(w, sse.Event{
					Id:    plcAddress,
					Event: "data",
					Data:  dane,
				})
				// Wysłanie i poczekanie
				w.Flush()

				// Czas ostatniego odczytu z PLC
				log.Println("Szybkość odczytu z PLC " + plcAddress + " " + strconv.FormatInt((readTimeEnd-readTimeStart)/1000000, 10) + " ms")

				// log.Println(plcAddress + " Wysłano: " + strconv.FormatInt(timestamp, 10))

				lastTime = time.Now().UnixNano()

				// log.Println(lastTime)
			}

			time.Sleep(time.Duration(period) * time.Millisecond)

			ix++
		}
	} else {

		log.Println("Odbebrałem niepoprawny adres IP: " + plcAddress)
		c.JSON(http.StatusOK, "Odbebrałem niepoprawny adres IP: "+plcAddress)
	}

	log.Println("LOOP end for PLC IP " + plcAddress)

	// // also a complex type, like a map, a struct or a slice
	// sse.Encode(w, sse.Event{
	// 	Id:    "124",
	// 	Event: "message",
	// 	Data: map[string]interface{}{
	// 		"user":    "manu",
	// 		"date":    time.Now().Unix(),
	// 		"content": "hi!",
	// 	},
	// })

}

// main - Program główny
// ================================================================================================
func main() {

	// SERVER HTTP
	// =======================================

	r := gin.Default()
	r.Use(Options)

	// r.LoadHTMLGlob("./dist/*.html")
	// r.StaticFS("/css", http.Dir("./dist/css"))
	// r.StaticFS("/js", http.Dir("./dist/js"))
	// r.StaticFile("/", "./dist/index.html")
	// r.StaticFile("favicon.ico", "./dist/favicon.ico")
	// r.GET("/api/v1/s7", S7Get)

	r.GET("/api/v1/data", SendData)
	r.GET("/api/v1/s7", eventHandler)

	r.Run(":80")
}

// // INFLUX
// // =======================================

// var myHTTPClient *http.Client

// influx, err := influxdb.New("http://localhost:9999", "_QpSsfqP7Z46od7XQSZAWpf3muEsesEYHR8LHVpMibiQMnlJm2dywKTbgveNhXtyvJKIMLgp14bARpUr8lzprQ==", influxdb.WithHTTPClient(myHTTPClient))
// if err != nil {
// 	panic(err) // error handling here; normally we wouldn't use fmt but it works for the example
// }

// // we use client.NewRowMetric for the example because it's easy, but if you need extra performance
// // it is fine to manually build the []client.Metric{}.
// myMetrics := []influxdb.Metric{
// 	influxdb.NewRowMetric(
// 		map[string]interface{}{"memory": 1000, "cpu": 0.93},
// 		"system-metrics",
// 		map[string]string{"hostname": "hal9000"},
// 		time.Date(2018, 3, 4, 5, 6, 7, 8, time.UTC)),
// 	influxdb.NewRowMetric(
// 		map[string]interface{}{"memory": 1000, "cpu": 0.93},
// 		"system-metrics",
// 		map[string]string{"hostname": "hal9000"},
// 		time.Date(2018, 3, 4, 5, 6, 7, 9, time.UTC)),
// }

// // The actual write..., this method can be called concurrently.
// if _, err := influx.Write(context.Background(), "iot2", "DTP", myMetrics...); err != nil {
// 	log.Fatal(err) // as above use your own error handling here.
// }
// influx.Close() // closes the client.  After this the client is useless.

// S7Get - Dane do połączenia
// // ================================================================================================
// func S7Get(c *gin.Context) {

// 	// Typ połączania
// 	c.Header("Access-Control-Allow-Origin", "*")
// 	// c.Header("Content-Type", "multipart/form-data")
// 	// c.Header("Connection", "Keep-Alive")
// 	// c.Header("Transfer-Encoding", "chunked")
// 	c.Header("X-Accel-Buffering", "no")

// 	plcAddress := c.Query("plc_address")
// 	slotNr, _ := strconv.Atoi(c.Query("slot_nr"))
// 	period, _ := strconv.Atoi(c.Query("period"))

// 	if net.ParseIP(plcAddress) != nil {

// 		// TCPClient
// 		handler := gos7.NewTCPClientHandler(plcAddress, 0, slotNr)
// 		handler.Timeout = time.Duration(period) * time.Millisecond
// 		handler.IdleTimeout = 5 * time.Second
// 		handler.PDULength = 960
// 		// handler.Logger = log.New(os.Stdout, "tcp: ", log.LstdFlags)

// 		// Connect manually so that multiple requests are handled in one connection session
// 		handler.Connect()
// 		defer handler.Close()

// 		client := gos7.NewClient(handler)

// 		bufEB := make([]byte, 128)
// 		bufMB := make([]byte, 128)

// 		client.AGReadEB(0, 128, bufEB)
// 		client.AGReadMB(0, 128, bufMB)

// 		var buf []byte
// 		for index := range bufMB {
// 			buf = append(buf, bufEB[index])
// 		}
// 		for index := range bufMB {
// 			buf = append(buf, bufMB[index])
// 		}

// 		c.Data(http.StatusOK, "multipart/form-data", buf)
// 	} else {
// 		log.Println("Odbebrałem niepoprawny adres IP: " + plcAddress)
// 		c.JSON(http.StatusOK, "Odbebrałem niepoprawny adres IP: "+plcAddress)

// 	}

// }
