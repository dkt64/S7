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

const startingPrecision = 1
const cyclesAnalyzeTime = 30
const cyclesAnalyzeTimeAdd = 20
const imageSize = 128 * 3
const minCycleTime = 10000

var plcAddress string
var plcConnected bool
var etap string
var comparePrecision int
var firstCycle bool
var cyclesTime int
var periodPrecision int64

// MachineImage - Rekord danych
// ========================================================
type MachineImage struct {
	Timestamp int64           `json:"Timestamp"`
	IOImage   [imageSize]byte `json:"IOImage"`
}

// Transision - Przejście między stanami
// Numer stanu z tablicy machineStates
// ========================================================
type Transision struct {
	StateNrSrc int   `json:"StateNrSrc"`
	StateNrDst int   `json:"StateNrDst"`
	Time       int64 `json:"Time"`
}

// Statistics - dane statystyczne
// ========================================================
type Statistics struct {
	Trans  []Transision      `json:"Trans"`
	Stats  []int             `json:"Stats"`
	States [][imageSize]byte `json:"States"`
}

// statistics - dane statystyczne
// ========================================================
var statistics Statistics

// Transisions - Tablica przejść między stanami
// ========================================================
var Transisions []Transision

// machineTimeline - Dane
// ========================================================
var machineTimeline []MachineImage

// machineStates - Dane
// ========================================================
var machineStates [][imageSize]byte

// cyclesFound - Znalezione cykle
// ========================================================
var cyclesFound []int64

// cyclesNrsFound - Numery obrazów które posłużyły za znalezienie cykli
// ========================================================
var cyclesNrsFound []int64

// maskImage - Maska obrazu - wybrane bajty nie są brane pod uwage przy rejestracji stanów maszyny
// ========================================================
var maskImage [imageSize]byte

// statesStatistics - Ile razy występuje stan z machinestates w timeline
// ========================================================
var statesStatistics []int

// machineStates - Dane
// ========================================================
var machineStatesNr int

// transisionNr - Dane
// ========================================================
var transisionNr int

// writeID - ostatnio anlizowany obraz w funkcji Write
// ========================================================
var writeID int

// stateNr - ostatnio anlizowany obraz w funkcji AnalyzeStatistics
// ========================================================
var stateNr int

// transID - ostatnio anlizowany obraz w funkcji Transisions
// ========================================================
var transID int

// valuesRange - Analiza zmienności danych
// ========================================================
var valuesRange [256][imageSize]byte

// conectionTimeStart - Czas rozpoczęcia analizy
// ========================================================
var conectionTimeStart int

//
// ImageZero - sprawdza czy obraz jest pusty
// ================================================================================================
func ImageZero(im1 [imageSize]byte) bool {

	empty := true
	for i := 0; i < imageSize; i++ {
		if im1[i] != 0 {
			empty = false
		}
	}

	return empty
}

//
// ImageEqual - Porównanie obrazów - jota w jotę
// ================================================================================================
func ImageEqual(im1 MachineImage, im2 MachineImage) bool {
	cnt := 0
	for i := 0; i < imageSize; i++ {
		if im1.IOImage[i] != im2.IOImage[i] {
			cnt++
		}
	}

	return cnt == 0
}

//
// ImageCompare - Zgodność obrazów
// ================================================================================================
func ImageCompare(im1 [imageSize]byte, im2 [imageSize]byte) int {

	cnt := 0
	for i := 0; i < imageSize; i++ {
		if im1[i] != im2[i] {
			cnt++
		}
	}

	return cnt
}

//
// MaskedImageEqual - Zgodność obrazów
// Parametr nr 1 - obraz maskowany
// Parametr nr 2 - obraz niemaskowany
// ================================================================================================
func MaskedImageEqual(imSrc [imageSize]byte, imMask [imageSize]byte) bool {

	cnt := 0
	for i := 0; i < imageSize; i++ {
		if imSrc[i] != (imMask[i] & maskImage[i]) {
			cnt++
		}
	}

	return cnt == 0
}

//
// MaskedImage - Zwraca obraz zamaskowany
// ================================================================================================
func MaskedImage(imSrc [imageSize]byte, imMask [imageSize]byte) [imageSize]byte {

	var imDst [imageSize]byte
	for i := 0; i < imageSize; i++ {
		imDst[i] = (imSrc[i] & imMask[i])
	}

	return imDst
}

//
// MaskedState - Zwraca obraz zamaskowany
// ================================================================================================
func MaskedState(imSrc MachineImage, imMask [imageSize]byte) MachineImage {

	imDst := imSrc
	for i := 0; i < imageSize; i++ {
		imDst.IOImage[i] = (imDst.IOImage[i] & imMask[i])
	}

	return imDst
}

//
// ImageDiff - Zwraca obraz z różnicami
// ================================================================================================
func ImageDiff(im1 MachineImage, im2 [imageSize]byte) [imageSize]byte {

	var im0 [imageSize]byte

	for i := 0; i < imageSize; i++ {
		if im1.IOImage[i] != im2[i] {
			im0[i] = 0
		} else {
			im0[i] = 0xff
		}
	}

	return im0
}

// ConnectionTime - czas połączenia
// ================================================================================================
func ConnectionTime() int {
	return int(time.Now().Unix()) - conectionTimeStart
}

// AnalyzeCycles - szukamy maksymalnego procenta wzrorca (największego obrazu który daje pattern)
// ================================================================================================
func AnalyzeCycles() {
	var patternFound bool
	var patternIndex1 int
	var patternIndex2 int
	var patternTimestamp1 int64
	var patternTimestamp2 int64
	var addCycle bool
	var cycleNrFound bool

	nrOfImages := len(machineTimeline)
	nrOfCyclesFound := 0

	for i, image1 := range machineTimeline {
		if i > 0 { // nie sprawdzamy obrazu pod indexem 0
			if !ImageEqual(image1, machineTimeline[i-1]) { // sprawdamy czy nastąpiła zmiana obrazu
				for j := 0; j < i; j++ {
					image2 := machineTimeline[j]

					comp := ImageCompare(image1.IOImage, image2.IOImage)
					if comp <= comparePrecision {

						patternIndex1 = i
						patternIndex2 = j
						patternTimestamp1 = image1.Timestamp
						patternTimestamp2 = image2.Timestamp
						// Drukuj jeżeli znaleźliśmy pattern powyżej 1000ms
						// Uwzględniamy tolerancję +/-500ms więc sprawdzamy w liście czy już takiego nie ma
						// Dodajemy do listy patternów

						newCycle := (patternTimestamp1 - patternTimestamp2) / 1000000 // milliseconds
						// log.Println("New cycle = " + strconv.FormatInt(newCycle, 10))
						addCycle = true
						for _, cycle := range cyclesFound {
							if (newCycle < (cycle + periodPrecision)) && (newCycle > (cycle - periodPrecision)) {
								addCycle = false
							}
						}
						cycleNrFound = true
						for _, nr := range cyclesNrsFound {
							if nr == int64(j) {
								cycleNrFound = false
								break
							}
						}

						if addCycle && cycleNrFound && newCycle > minCycleTime {
							patternFound = true
							cyclesFound = append(cyclesFound, newCycle)
							cyclesNrsFound = append(cyclesNrsFound, int64(j))

							log.Println("Pattern found (" +
								strconv.Itoa(comp) + " bytes precision) with duration " +
								strconv.FormatInt(newCycle, 10) + " [ms] at indexes [" +
								strconv.Itoa(patternIndex1) + "][" +
								strconv.Itoa(patternIndex2) + "]")

							log.Println("images nrs for cycles:")
							log.Println(cyclesNrsFound)

							// gdy jest to pierwszy napotkany wzorzec zapisujemy maskę
							if nrOfCyclesFound == 0 && !firstCycle {
								maskImage = ImageDiff(image1, image2.IOImage)
								firstCycle = true
								log.Println("Mask image:")
								log.Println(maskImage)
							}

							// log.Println("image1:")
							// log.Println(image1.IOImage)
							// log.Println("image2:")
							// log.Println(image2.IOImage)

							nrOfCyclesFound++
						}
						break
					}
				}
			}
		}
		if nrOfCyclesFound >= 10 {
			break
		}
	}

	if !patternFound {
		log.Println("Pattern not found in " + strconv.Itoa(nrOfImages) + " machine states records, precision = " + strconv.Itoa(comparePrecision))
		// log.Println("Mask image:")
		// log.Println(maskImage)
	} else {
		log.Println("Pattern found in " + strconv.Itoa(nrOfImages) + " machine states records")
		if addCycle {
			log.Println("Cycles list:")
			log.Println(cyclesFound)
		}
		// log.Println("Mask image:")
		// log.Println(maskImage)
	}
}

// AnalyzeWrite - zapis tylko nowych obrazów
// ================================================================================================
func AnalyzeWrite() {

	// var maskedImage [imageSize]byte

	length := len(machineTimeline)
	for i := writeID; i < length; i++ {
		// maskujemy obraz
		maskedImage := MaskedImage(machineTimeline[i].IOImage, maskImage)
		// sprawdzamy czy już taki mamy
		newImage := true
		for _, image2 := range machineStates {
			if ImageCompare(maskedImage, image2) == 0 {
				newImage = false
				break
			}
		}
		// jeżeli nowy i nie zerowy
		if newImage && !ImageZero(maskedImage) {
			// dodajemy do listy stanów
			machineStates = append(machineStates, maskedImage)
			// dodajemy również do statystyk
			statesStatistics = append(statesStatistics, 0)
			machineStatesNr++
			log.Println("New image registered nr " + strconv.Itoa(len(machineStates)))
			log.Println(maskedImage)
		}
	}
	writeID = length
	log.Println(machineStatesNr, "images registered")
}

// AnalyzeStatistics - update ilości występowania state w transisions
// ================================================================================================
func AnalyzeStatistics() {

	length := len(machineTimeline) - 1
	for _, trans := range Transisions {
		for j := stateNr; j < length; j++ {
			image1 := MaskedState(machineTimeline[j], maskImage)
			image2 := MaskedState(machineTimeline[j+1], maskImage)

			if ImageCompare(machineStates[trans.StateNrSrc], image1.IOImage) == 0 &&
				ImageCompare(machineStates[trans.StateNrDst], image2.IOImage) == 0 {
				statesStatistics[trans.StateNrSrc]++
				statesStatistics[trans.StateNrDst]++
			}
		}
	}
	stateNr = length
	log.Println("States statistics ", statesStatistics)
}

// AnalyzeTransitions - zapis przejść
// ================================================================================================
func AnalyzeTransitions() {

	length := len(machineTimeline) - 2

	for i := transID; i < length; i++ {

		// pobierz obrazy z timeline
		// działamy na obrazach zamaskowanych

		image0 := MaskedState(machineTimeline[i], maskImage)
		imageSrc := MaskedState(machineTimeline[i+1], maskImage)

		// szukanie pierwszej zmiany stanu
		if !ImageEqual(image0, imageSrc) {

			// szukanie drugiej zmiany stanu
			for i := i; i < length; i++ {

				// pobierz obrazy z timeline
				image1 := MaskedState(machineTimeline[i+1], maskImage)
				imageDst := MaskedState(machineTimeline[i+2], maskImage)

				// kolejna zmiana stanu
				if !ImageEqual(image1, imageDst) {

					var srcIndex int
					var dstIndex int

					// szkamy numerów stanów w tablicy stanów
					for k, state := range machineStates {
						if ImageCompare(state, imageSrc.IOImage) == 0 {
							srcIndex = k
							break
						}
					}
					for k, state := range machineStates {
						if ImageCompare(state, imageDst.IOImage) == 0 {
							dstIndex = k
							break
						}
					}

					if srcIndex != dstIndex {

						period1 := (imageDst.Timestamp - imageSrc.Timestamp) / 1000000

						// sprawdzamy czy jest taka kompinacja w transitions

						newTrans := true
						for _, trans := range Transisions {
							period2 := trans.Time
							if trans.StateNrSrc == srcIndex && trans.StateNrDst == dstIndex && period1 > period2-periodPrecision && period1 < period2+periodPrecision {
								newTrans = false
								break
							}
						}
						if newTrans {
							Transisions = append(Transisions,
								Transision{
									StateNrSrc: srcIndex,
									StateNrDst: dstIndex,
									Time:       period1,
								})
							// log.Println("New transision registered from", srcIndex, "to", dstIndex, "with period", period1)
							transisionNr++
						}
					}
					// koniec - nie szukamy kolejnych zmian
					break
				}
			}
		}
	}
	transID = length
	log.Println(transisionNr, "transitions registered")
}

//
// ScanTimeline - Analiza
//
// 1) Szukanie maksymalnego procentu cyklu - zaczynamy od 100% i schodzimy o jeden bajt w dół
// 2) Zapisywanie obrazów do osobnej tablicy i dodawać tylko te nowe
// 3) Stworzyć tablicę przejść z czasem przejścia (graf stanów)
// 4) Uwzględnić tolerancję czasu - nie rejestrować cykli podobnych, gdyż może to wynikać samej komunikacji
// 5) Zapisać obrazy dla których wykryte zostały cykle aby nie dodawać nowych które już mamy w bazie
// ================================================================================================
func ScanTimeline() {

	for {
		if plcConnected {
			switch etap {

			case "AnalyzeCycles":
				AnalyzeCycles()
				if ConnectionTime() >= cyclesTime {
					if len(cyclesFound) == 0 {
						log.Println("Didn't found any cycles with precision " + strconv.Itoa(comparePrecision))
						comparePrecision++
						cyclesTime += cyclesAnalyzeTimeAdd
						log.Println("Decreasing precision to " + strconv.Itoa(comparePrecision) + " bytes")
					} else {
						etap = "AnalyzeWrite"
						log.Println("AnalyzeCycles -> AnalyzeWrite...")
					}
				}
			case "AnalyzeWrite":
				// AnalyzeCycles()
				AnalyzeWrite()
				AnalyzeTransitions()
				AnalyzeStatistics()
			default:
				conectionTimeStart = int(time.Now().Unix())
				if plcConnected {
					etap = "AnalyzeCycles"
					log.Println("default -> AnalyzeCycles...")
				}
			}
			time.Sleep(5000 * time.Millisecond)

			log.Println(etap, "time", ConnectionTime(), "/", cyclesTime)
		} else {
			InitVars()
			etap = "waiting"
			log.Println("Waiting for connection...")
			time.Sleep(5000 * time.Millisecond)
		}
	}
}

// InitVars - reset tablic i stanów
// ================================================================================================
func InitVars() {
	machineTimeline = nil
	machineStates = nil
	cyclesFound = nil
	cyclesNrsFound = nil
	Transisions = nil
	statesStatistics = nil
	statistics.States = nil
	statistics.Stats = nil
	statistics.Trans = nil
	machineStatesNr = 0
	transisionNr = 0
	writeID = 0
	transID = 0
	stateNr = 0
	firstCycle = false
	comparePrecision = startingPrecision
	cyclesTime = cyclesAnalyzeTime
	periodPrecision = 100
	etap = "waiting"
}

// base64Encode
// ================================================================================================
func base64Encode(str string) string {
	return base64.StdEncoding.EncodeToString([]byte(str))
}

// base64Decode
// ================================================================================================
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

//
// SendData - Wysłanie całej tablicy
// ================================================================================================
func SendData(c *gin.Context) {
	// Typ połączania
	c.Header("Access-Control-Allow-Origin", "*")
	log.Println("SendData()")

	// log.Println(machineTimeline[0].Timestamp)

	statistics.States = machineStates
	statistics.Stats = statesStatistics
	statistics.Trans = Transisions
	data, _ := json.MarshalIndent(statistics, "", "  ")

	log.Println(string(data))
	c.JSON(http.StatusOK, string(data))

	// data1, _ := json.Marshal(maskImage)
	// data2, _ := json.Marshal(machineStates)
	// data3, _ := json.Marshal(Transisions)
	// data4, _ := json.Marshal(statesStatistics)

	// var data []byte
	// data = append(data, data1...)
	// data = append(data, data2...)
	// data = append(data, data3...)
	// data = append(data, data4...)
	// // log.Println(string(data))

	// c.JSON(http.StatusOK, string(data))
}

//
// eventHandler - Zdarzenia
// ================================================================================================
func eventHandler(c *gin.Context) {
	// func eventHandler(w http.ResponseWriter, req *http.Request) {

	plcAddress := c.Query("plc_address")
	slotNr, _ := strconv.Atoi(c.Query("slot_nr"))
	period, _ := strconv.Atoi(c.Query("period"))

	InitVars()

	for cval := 0; cval < 256; cval++ {
		for cindex := 0; cindex < imageSize; cindex++ {
			valuesRange[cval][cindex] = 0
		}
	}

	if net.ParseIP(plcAddress) != nil {
		log.Println("Odbebrałem adres IP: " + plcAddress)

		// TCPClient
		handler := gos7.NewTCPClientHandler(plcAddress, 0, slotNr)
		handler.Timeout = 5 * time.Second
		handler.IdleTimeout = 5 * time.Second
		handler.PDULength = 960

		// handler.Logger = log.New(os.Stdout, plcAddress+" : ", log.LstdFlags)

		// Connect manually so that multiple requests are handled in one connection session
		ret := handler.Connect()
		defer handler.Close()
		// log.Println(ret)
		client := gos7.NewClient(handler)
		// log.Println(client)

		log.Println("Wynegocjowany PDU length =", handler.PDULength)

		if ErrCheck(ret) {
			plcConnected = true
			defer func() {
				plcConnected = false
			}()

			bufMB := make([]byte, 128)
			bufEB := make([]byte, 128)
			bufAB := make([]byte, 128)

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

			go func() {
				<-clientGone
				plcConnected = false
			}()

			log.Println("LOOP start for PLC IP " + plcAddress + " ...")

			var ix int
			lastTime := time.Now().UnixNano()
			lastTime2 := time.Now().UnixNano()
			lastTime3 := time.Now().UnixNano()

			for {

				// Jeżeli połączenie zamknięte to break
				if !plcConnected {
					log.Println("Client Gone...")
					break
				}

				// Odczyt sygnałów z PLC
				readTimeStart := time.Now().UnixNano()

				// Jeżeli PDU Length większy/równy od rozmiaru obrazu to odczytujemy wszystko razem
				if handler.PDULength >= imageSize {
					var error1, error2, error3 string

					var items = []gos7.S7DataItem{
						gos7.S7DataItem{
							Area:    0x81,
							WordLen: 0x02,
							Start:   0,
							Amount:  128,
							Data:    bufEB,
							Error:   error1,
						},
						gos7.S7DataItem{
							Area:    0x82,
							WordLen: 0x02,
							Start:   0,
							Amount:  128,
							Data:    bufAB,
							Error:   error2,
						},
						gos7.S7DataItem{
							Area:    0x83,
							WordLen: 0x02,
							Start:   0,
							Amount:  128,
							Data:    bufMB,
							Error:   error3,
						},
					}
					err := client.AGReadMulti(items, 3)
					ErrCheck(err)

				} else {
					client.AGReadMB(0, 128, bufMB)
					client.AGReadEB(0, 128, bufEB)
					client.AGReadAB(0, 128, bufAB)
				}

				readTimeEnd := time.Now().UnixNano()

				if bufMB == nil || bufEB == nil || bufAB == nil {
					log.Println("NIL...")
					break
				}

				var buf [imageSize]byte
				for index := range bufMB {
					buf[index+128*0] = bufMB[index]
				}
				for index := range bufEB {
					buf[index+128*1] = bufEB[index]
				}
				for index := range bufEB {
					buf[index+128*2] = bufAB[index]
				}

				// sprawdzamy czy same zera - jak tak to nie zapisujemy
				emptyBuf := true
				for i := range buf {
					if buf[i] > 0 {
						emptyBuf = false
					}
				}

				if emptyBuf {
					log.Println("Pusty bufor!?")
					plcConnected = false
				} else {

					// Dodajemy do timeline
					// ==============================================

					machineTimeline = append(machineTimeline, MachineImage{Timestamp: readTimeEnd, IOImage: buf})

					// Dodajemy do valuesRange
					// ==============================================

					for cval := 0; cval < 256; cval++ {
						for cindex := 0; cindex < imageSize; cindex++ {
							if buf[cindex] == byte(cval) {
								if valuesRange[cval][cindex] < 255 {
									valuesRange[cval][cindex]++
								}
							}
						}
					}

					// Wysyłamy timeline do VISU co 500 ms (ekran PLC)
					// ==============================================

					dane := map[string]interface{}{
						"time":    readTimeEnd,
						"content": buf,
					}

					if readTimeEnd-lastTime > 500000000 {

						sse.Encode(w, sse.Event{
							Id:    plcAddress,
							Event: "data",
							Data:  dane,
						})
						// Wysłanie i poczekanie
						w.Flush()

						// log.Println(plcAddress + " Wysłano: " + strconv.FormatInt(timestamp, 10))

						lastTime = time.Now().UnixNano()

						// log.Println(lastTime)
					}

					// Wysyłamy ranges do VISU co 5000 ms (ekran PLC)
					// ==============================================

					rangesTab := map[string]interface{}{
						// "time":    readTimeEnd,
						"content": valuesRange,
					}

					cyclesTab := map[string]interface{}{
						// "time":    readTimeEnd,
						"content": cyclesFound,
					}

					if readTimeEnd-lastTime2 > 5000000000 {

						sse.Encode(w, sse.Event{
							Id:    plcAddress,
							Event: "stats",
							Data:  rangesTab,
						})
						// Wysłanie i poczekanie
						w.Flush()

						// Czas ostatniego odczytu z PLC
						log.Println("Szybkość ostatniego odczytu danych z PLC " + plcAddress + " " + strconv.FormatInt((readTimeEnd-readTimeStart)/1000000, 10) + " ms")

						// log.Println("Wysyłam tablicę zmian...")

						lastTime2 = time.Now().UnixNano()
					}

					// Wysyłamy listę cykli co 5000 ms
					// ==============================================

					if readTimeEnd-lastTime3 > 5000000000 {

						sse.Encode(w, sse.Event{
							Id:    plcAddress,
							Event: "cycles",
							Data:  cyclesTab,
						})
						// Wysłanie i poczekanie
						w.Flush()

						// log.Println("Wysyłam tablicę cykli...")

						lastTime3 = time.Now().UnixNano()
					}

					time.Sleep(time.Duration(period) * time.Millisecond)

					// licznik
					ix++
				}

			}
		} else {
			log.Println("Problem z połączeniem z " + plcAddress)
			c.JSON(http.StatusOK, "Problem z połączeniem z "+plcAddress)
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

	plcConnected = false

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

	// Odpalenie drugiego wątku analizy danych
	go ScanTimeline()

	r.Run(":80")
}
