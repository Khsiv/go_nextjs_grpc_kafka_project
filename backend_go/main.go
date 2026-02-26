package main

import (
	"api/controller"
	"log"
	"net/http"
)

// main function
func main() {

	// create router
	router := controller.GetRouter()

	// start server
	log.Fatal(http.ListenAndServe(":8000", router))
}
