package main

import (
	"encoding/base64"
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

// // readData - Odczyt DB
// // ================================================================================================
// func readData(dbNr int, startAddress int, dataSize int) {

// 	const (
// 		// tcpDevice = "192.168.1.10" // NetLink
// 		tcpDevice = "192.168.0.1" // S7-315 PN
// 		rack      = 0
// 		slot      = 2
// 	)

// 	// TCPClient
// 	handler := gos7.NewTCPClientHandler(tcpDevice, rack, slot)
// 	handler.Timeout = 5 * time.Second
// 	handler.IdleTimeout = 5 * time.Second
// 	// handler.PDULength = 1024
// 	// handler.Logger = log.New(os.Stdout, "tcp: ", log.LstdFlags)

// 	// Connect manually so that multiple requests are handled in one connection session
// 	handler.Connect()
// 	defer handler.Close()

// 	client := gos7.NewClient(handler)
// 	buf := make([]byte, dataSize)

// 	for {
// 		client.AGReadDB(dbNr, startAddress, dataSize, buf)
// 		// client.AGReadEB(0, 128, buf)
// 		// client.AGReadAB(0, 128, buf)
// 		// fmt.Println(startAddress)
// 	}
// }

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

// S7Get - Dane do połączenia
// ================================================================================================
func S7Get(c *gin.Context) {

	// Typ połączania
	c.Header("Access-Control-Allow-Origin", "*")
	// c.Header("Content-Type", "multipart/form-data")
	// c.Header("Connection", "Keep-Alive")
	// c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Accel-Buffering", "no")

	plcAddress := c.Query("plc_address")
	slotNr, _ := strconv.Atoi(c.Query("slot_nr"))
	period, _ := strconv.Atoi(c.Query("period"))

	if net.ParseIP(plcAddress) != nil {
		// log.Println("Odbebrałem adres IP: " + plcAddress)

		// tcpDevice = "192.168.1.10" // NetLink
		// const (
		// 	rack = 0
		// 	slot = slotNr
		// )

		// TCPClient
		handler := gos7.NewTCPClientHandler(plcAddress, 0, slotNr)
		handler.Timeout = time.Duration(period) * time.Millisecond
		handler.IdleTimeout = 5 * time.Second
		handler.PDULength = 960
		// handler.Logger = log.New(os.Stdout, "tcp: ", log.LstdFlags)

		// Connect manually so that multiple requests are handled in one connection session
		handler.Connect()
		defer handler.Close()

		client := gos7.NewClient(handler)
		bufEB := make([]byte, 128)
		bufMB := make([]byte, 128)

		// w := c.Writer
		// clientGone := w.CloseNotify()

		// Streaming LOOP...
		// ----------------------------------------------------------------------------------------------

		// for {

		// 	// Jeżeli straciimy kontekst to wychodzimy
		// 	if c.Request.Context() == nil {
		// 		log.Println("ERR! c.Request.Context() == nil")
		// 		break
		// 	}

		// if <-clientGone {
		// 	log.Println("Client Gone...")
		// 	break
		// }

		client.AGReadEB(0, 128, bufEB)
		client.AGReadMB(0, 128, bufMB)

		var buf []byte
		for index := range bufMB {
			buf = append(buf, bufEB[index])
		}
		for index := range bufMB {
			buf = append(buf, bufMB[index])
		}

		c.Data(http.StatusOK, "multipart/form-data", buf)
		// w.Write(buf)
		// w.Flush()

		// c.JSON(http.StatusOK, buf)

		// log.Println(buf)

		// log.Println(bufMB)
		// c.JSON(http.StatusOK, "OK")

		// time.Sleep(200 * time.Millisecond)
		// }

		// // Feedback gdybyśmy wyszli z LOOP
		// log.Println("Loop ended.")
		// c.JSON(http.StatusOK, "Loop ended.")

	} else {
		log.Println("Odbebrałem niepoprawny adres IP: " + plcAddress)
		c.JSON(http.StatusOK, "Odbebrałem niepoprawny adres IP: "+plcAddress)

	}

}

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

		log.Println("eventHandler")
		c.JSON(http.StatusOK, "eventHandler")

		w := c.Writer

		clientGone := w.CloseNotify()
		var closed bool

		go func() {
			<-clientGone
			closed = true
		}()

		// data can be a primitive like a string, an integer or a float
		var ix int
		// for ix = 0; ix < 40; ix++ {
		for {

			client.AGReadEB(0, 128, bufEB)
			client.AGReadMB(0, 128, bufMB)

			var buf []byte
			for index := range bufMB {
				buf = append(buf, bufEB[index])
			}
			for index := range bufMB {
				buf = append(buf, bufMB[index])
			}

			timestamp := time.Now().UnixNano()

			// sse.Encode(w, sse.Event{
			// 	Event: "data",
			// 	Data:  "event nr " + strconv.Itoa(ix),
			// })
			// log.Println("event nr " + strconv.Itoa(ix))

			sse.Encode(w, sse.Event{
				Id:    plcAddress,
				Event: "data",
				Data: map[string]interface{}{
					"time":    timestamp,
					"content": buf,
				},
			})
			log.Println(plcAddress + ": " + strconv.FormatInt(timestamp, 10))

			w.Flush()

			time.Sleep(time.Duration(period) * time.Millisecond)

			if closed {
				log.Println("Client Gone...")
				break
			}

			ix++
		}
	} else {

		log.Println("Odbebrałem niepoprawny adres IP: " + plcAddress)
		c.JSON(http.StatusOK, "Odbebrałem niepoprawny adres IP: "+plcAddress)
	}

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

	r := gin.Default()
	r.Use(Options)

	// r.LoadHTMLGlob("./dist/*.html")

	// r.StaticFS("/css", http.Dir("./dist/css"))
	// r.StaticFS("/js", http.Dir("./dist/js"))

	// r.StaticFile("/", "./dist/index.html")
	// r.StaticFile("favicon.ico", "./dist/favicon.ico")

	// r.GET("/api/v1/s7", S7Get)
	r.GET("/api/v1/s7", eventHandler)

	r.Run(":80")
}
