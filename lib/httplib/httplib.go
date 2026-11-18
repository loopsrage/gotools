package httplib

import "net/http"

func WriteResponse(w http.ResponseWriter, message *[]byte, status *int) {
	w.WriteHeader(*status)
	w.Write(*message)
}

func WriteCorsResponse(w http.ResponseWriter, message *[]byte, status *int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "*")
	WriteResponse(w, message, status)
}
