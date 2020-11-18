package main

import (
	"database/sql"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var db *sql.DB

func init() {
	tmpDB, err := sql.Open("postgres", "dbname=books_database user=postgres password=changeme host=192.168.122.40 sslmode=disable")
	if err != nil {
		log.Fatal(err)
	}
	db = tmpDB
}

func main() {
	finish := make(chan bool)
	servermain := http.NewServeMux()
	servermain.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("www/assets"))))
	servermain.HandleFunc("/", handleListBooks)
	servermain.HandleFunc("/book.html", handleViewBook)
	servermain.HandleFunc("/save", handleSaveBook)
	servermain.HandleFunc("/delete", handleDeleteBook)

	serverprometheus := http.NewServeMux()
	serverprometheus.Handle("/metrics", promhttp.Handler())

	go func() {
		http.ListenAndServe(":8080", servermain)
		log.Fatal(http.ListenAndServe(":8080", nil))
	}()

	go func() {
		http.ListenAndServe(":2112", serverprometheus)
		log.Fatal(http.ListenAndServe(":2112", nil))
	}()

	<-finish
}
