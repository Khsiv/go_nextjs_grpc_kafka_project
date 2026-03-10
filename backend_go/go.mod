module api

go 1.25.5

require (
	backend_grpc v0.0.0
	github.com/gorilla/mux v1.8.1
	github.com/lib/pq v1.11.2
	golang.org/x/oauth2 v0.34.0
	google.golang.org/grpc v1.79.2
)

require (
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace backend_grpc => ../backend_grpc
