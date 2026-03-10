package main

import (
	"api/controller"
	"log"
	"net/http"
)

// main function
func main() {

	db := controller.InitDatabase()
	defer db.Close()

	grpcCLient, conn := controller.InitGrpcClient()
	defer conn.Close()

	// create router
	router := controller.GetRouter(db, grpcCLient)

	// start server
	log.Fatal(http.ListenAndServe(":8000", router))
}
