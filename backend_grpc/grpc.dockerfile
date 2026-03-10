FROM golang:1.25.5-alpine3.22

WORKDIR /app

COPY . .

RUN go mod download
RUN go build -o server_grpc .

EXPOSE 50051

CMD ["./server_grpc"]