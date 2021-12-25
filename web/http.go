package main

import (
  "net/http"
  "log"
  "github.com/gorilla/mux"
)

func httpHandler (w http.ResponseWriter, r *http.Request) {
  http.ServeFile(w, r, http.Dir("./index.html"))
}

func main() {
  r := mux.NewRouter()
  r.HandleFunc("/", httpHandler)

  log.Fatal(http.ListenAndServe("127.0.0.1:80", r))
}
