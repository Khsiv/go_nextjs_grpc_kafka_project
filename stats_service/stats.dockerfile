# Built from the repository root so the shared events module is available too.
FROM golang:1.25.5-alpine3.22

WORKDIR /app

COPY stats_service/ ./stats_service/
COPY events/ ./events/

WORKDIR /app/stats_service

RUN go mod download

RUN go build -o stats_service .

CMD ["./stats_service"]
