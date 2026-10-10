module api

go 1.25.5

require (
	backend_grpc v0.0.0
	github.com/gorilla/mux v1.8.1
	github.com/lib/pq v1.11.2
	github.com/segmentio/kafka-go v0.4.51
	golang.org/x/crypto v0.46.0
	golang.org/x/oauth2 v0.34.0
	google.golang.org/grpc v1.79.2
)

require (
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
)

require (
	cloud.google.com/go/compute/metadata v0.9.0 // indirect
	events v0.0.0
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
	golang.org/x/text v0.32.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

replace backend_grpc => ../backend_grpc

replace events => ../events
