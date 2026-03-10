package main

import (
	"backend_grpc/pb"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"os"

	_ "github.com/lib/pq"
	"google.golang.org/grpc"
)

type Server struct {
	pb.UnimplementedGreeterServer
	db *sql.DB
}

func (s *Server) MakePost(ctx context.Context, message *pb.MeesagePost) (*pb.Status, error) {

	var returnId int
	fmt.Println("in MakePost")
	err := s.db.QueryRow("INSERT INTO posts (users_id, post_text) VALUES ($1, $2)  RETURNING id", message.UserId, message.Content).Scan(&returnId)
	if err != nil {
		log.Fatal(err)
		return &pb.Status{Status: false}, errors.New("error happens during inserting in db")
	}

	return &pb.Status{Status: true}, nil
}

func NewServer(db *sql.DB) *Server {
	return &Server{db: db}
}

func main() {

	connStr := os.Getenv("DATABASE_URL")
	if connStr == "" {
		connStr = "postgres://postgres:postgres@localhost:5433/postgres?sslmode=disable"
		log.Println("DATABASE_URL not set, using default local connection:", connStr)
	}

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatal(err)
	}

	defer db.Close()

	lis, err := net.Listen("tcp", ":50051")
	if err != nil {
		log.Fatal(err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterGreeterServer(grpcServer, NewServer(db))

	log.Println("gRPC server listening on :50051")
	log.Fatal(grpcServer.Serve(lis))

}
