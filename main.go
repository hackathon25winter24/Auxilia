package main

import (
	"log"
	"net/http"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	cfgpkg "auxilia/config"
	handlergrpc "auxilia/handler/grpc"
	httpserver "auxilia/handler/http"
	gormdb "auxilia/infrastructure/gorm"
	"auxilia/pb"
)

func main() {
	cfg, err := cfgpkg.LoadConfig()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	db, err := gormdb.NewGormDB(cfg)
	if err != nil {
		log.Fatalf("failed to connect to DB: %v", err)
	}

	s := grpc.NewServer()
	pb.RegisterUserServiceServer(s, handlergrpc.NewServer(db))
	reflection.Register(s)

	httpHandler := httpserver.NewHandler(s)
	httpServer := &http.Server{
		Addr:    ":" + cfg.AppPort,
		Handler: httpHandler,
	}

	log.Printf("Server listening at %s", cfg.AppPort)
	if err := httpServer.ListenAndServe(); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}