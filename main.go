package main

import (
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

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
	c.Header("Connection", "Keep-Alive")
	c.Header("Transfer-Encoding", "chunked")

	plcAddress := c.Query("plc_address")

	if net.ParseIP(plcAddress) != nil {
		log.Println("Odbebrałem adres IP: " + plcAddress)

		// tcpDevice = "192.168.1.10" // NetLink
		const (
			rack = 0
			slot = 2
		)

		// TCPClient
		handler := gos7.NewTCPClientHandler(plcAddress, rack, slot)
		handler.Timeout = 5 * time.Second
		handler.IdleTimeout = 5 * time.Second
		handler.PDULength = 960
		// handler.Logger = log.New(os.Stdout, "tcp: ", log.LstdFlags)

		// Connect manually so that multiple requests are handled in one connection session
		handler.Connect()
		defer handler.Close()

		client := gos7.NewClient(handler)
		bufEB := make([]byte, 128)
		bufMB := make([]byte, 128)

		// Streaming LOOP...
		// ----------------------------------------------------------------------------------------------

		// for {

		// // Wysyłamy pakiet co 100 ms
		// time.Sleep(200 * time.Millisecond)

		// // Jeżeli straciimy kontekst to wychodzimy
		// if c.Request.Context() == nil {
		// 	log.Println("ERR! c.Request.Context() == nil")
		// 	break
		// }

		client.AGReadEB(0, 128, bufEB)
		client.AGReadMB(0, 128, bufMB)

		var buf []byte
		// example
		// to combine two slices or join arrays, use for loop and builtin append function
		for index := range bufMB {
			buf = append(buf, bufEB[index])
		}
		for index := range bufMB {
			buf = append(buf, bufMB[index])
		}

		c.Data(http.StatusOK, "multipart/form-data", buf)
		// c.JSON(http.StatusOK, buf)

		log.Println(buf)

		// log.Println(bufMB)
		// c.JSON(http.StatusOK, "OK")

		// }

		// Feedback gdybyśmy wyszli z LOOP
		log.Println("Loop ended.")
		// c.JSON(http.StatusOK, "Loop ended.")

	} else {
		log.Println("Odbebrałem niepoprawny adres IP: " + plcAddress)
		c.JSON(http.StatusOK, "Odbebrałem niepoprawny adres IP: "+plcAddress)

	}

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

	api := r.Group("/api/v1")
	{
		api.GET("/s7", S7Get)
	}

	r.Run(":80")
}
