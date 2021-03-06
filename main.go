package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"rsc.io/qr"
)

// WIFI:S:<SSID>;T:<WPA|WEP|>;P:<password>;

type encryption string

const (
	encNone encryption = ""
	encWEP             = "WEP"
	encWPA             = "WPA"
	encWPA2            = "WPA2"
)

var errBadEncryption = errors.New("bad encryption mode")

func (e *encryption) UnmarshalJSON(b []byte) error {
	if len(b) < 2 || b[0] != '"' || b[len(b)-1] != '"' {
		return errBadEncryption
	}

	switch encryption(string(b[1 : len(b)-1])) {
	case encNone:
		*e = encNone
	case encWEP:
		*e = encWEP
	case encWPA, encWPA2:
		*e = encWPA
	default:
		return errBadEncryption
	}

	return nil
}

type wifi struct {
	SSID     string     `json:"ssid"`
	Enc      encryption `json:"enc"`
	Password string     `json:"password"`
}

func (w *wifi) QRText() string {
	return fmt.Sprintf("WIFI:S:%s;:T:%s;P:%s;;", w.SSID, w.Enc, w.Password)
}

var dbmu sync.RWMutex

// TODO(dgryski): persist via boltdb
var db = make(map[string]wifi)

var bOK = []byte("OK")

func wifiHandler(w http.ResponseWriter, r *http.Request) {

	if r.Method == "POST" {
		var wi wifi
		dec := json.NewDecoder(r.Body)
		err := dec.Decode(&wi)
		if err != nil {
			log.Printf("error decoding: %+v\n", err)
			http.Error(w, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		dbmu.Lock()
		db[wi.SSID] = wi
		dbmu.Unlock()

		w.Write(bOK)
		return
	}

	ssid := r.FormValue("ssid")

	dbmu.RLock()
	wifi, ok := db[ssid]
	dbmu.RUnlock()

	if !ok {
		http.NotFound(w, r)
		return
	}

	jenc := json.NewEncoder(w)
	jenc.Encode(wifi)
}

func updateHandler(w http.ResponseWriter, r *http.Request) {

	ssid := r.FormValue("ssid")

	dbmu.RLock()
	wifi, ok := db[ssid]
	dbmu.RUnlock()

	if !ok {
		http.NotFound(w, r)
		return
	}

	pass := r.FormValue("password")

	wifi.Password = pass

	dbmu.Lock()
	db[wifi.SSID] = wifi
	dbmu.Unlock()

	w.Write(bOK)
}

func qrHandler(w http.ResponseWriter, r *http.Request) {

	uri := r.RequestURI

	// TODO(dgryski): handle /qr/ssid for an html page that renders the QR code with also instructions

	if !strings.HasPrefix(uri, "/qr/") || !strings.HasSuffix(uri, ".png") {
		http.NotFound(w, r)
		return
	}

	ssid := uri[len("/qr/") : len(uri)-len(".png")]

	dbmu.RLock()
	wifi, ok := db[ssid]
	dbmu.RUnlock()

	if !ok {
		http.NotFound(w, r)
		return
	}

	text := wifi.QRText()

	code, err := qr.Encode(text, qr.Q)
	if err != nil {
		log.Printf("error encoding: %q: %v", text, err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-type", "image/png")
	w.Write(code.PNG())
}

func main() {

	port := flag.Int("p", 8080, "port")

	flag.Parse()

	if p := os.Getenv("PORT"); p != "" {
		*port, _ = strconv.Atoi(p)
	}

	// TODO(dgryski): need handler on / with instructions
	http.HandleFunc("/qr/", qrHandler)
	// TODO(dgryski): allow listing, deleting access points
	http.HandleFunc("/wifi", wifiHandler)
	// TODO(dgryski): add a bookmarklet for use with update
	http.HandleFunc("/update", updateHandler)

	log.Println("listening on port", *port)
	log.Fatalln(http.ListenAndServe(":"+strconv.Itoa(*port), nil))
}
