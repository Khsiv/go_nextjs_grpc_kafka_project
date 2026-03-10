# use official Golang image
FROM golang:1.25.5-alpine3.22

# set working directory
WORKDIR /app

# Copy the source code
# COPY . .
COPY backend_go/ ./backend_go/
COPY backend_grpc/ ./backend_grpc/

WORKDIR /app/backend_go

# Download and install the dependencies
RUN go get -d -v ./...
# RUN go mod download

# Build the Go app
RUN go build -o api .

#EXPOSE the port
EXPOSE 8000

# Run the executable
CMD ["./api"]