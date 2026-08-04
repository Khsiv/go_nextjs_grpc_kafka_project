module stats_service

go 1.25.5

require (
	github.com/lib/pq v1.11.2
	github.com/segmentio/kafka-go v0.4.51
)

require (
	events v0.0.0
	github.com/klauspost/compress v1.15.9 // indirect
	github.com/pierrec/lz4/v4 v4.1.15 // indirect
)

replace events => ../events
