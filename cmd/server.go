package main

import (
	"bytes"
	"encoding/json"
	"github.com/gorilla/mux"
	"log"
	"net/http"
)

type video struct {
	ID string `json:"id"`
}

func readBody(r *http.Request) string {
	buf := new(bytes.Buffer)
	buf.ReadFrom(r.Body)
	return buf.String()
}
func download(w http.ResponseWriter, r *http.Request) {
	youTubeUrl := readBody(r)
	json.NewEncoder(w).Encode(video{ID: youTubeUrl})
}
func main() {
	router := mux.NewRouter()
	router.HandleFunc("/video", download).Methods("POST")
	log.Fatal(http.ListenAndServe(":8000", router))
}
