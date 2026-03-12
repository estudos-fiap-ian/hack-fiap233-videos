package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/hack-fiap233/videos/internal/adapter/event"
	handler "github.com/hack-fiap233/videos/internal/adapter/http"
	"github.com/hack-fiap233/videos/internal/adapter/repository"
	"github.com/hack-fiap233/videos/internal/adapter/storage"
	"github.com/hack-fiap233/videos/internal/application"
	"github.com/hack-fiap233/videos/internal/middleware"
	_ "github.com/lib/pq"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	db := mustConnectDB()
	repo := repository.NewPostgresRepository(db)
	if err := repo.CreateTable(context.Background()); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	svc := application.NewVideoService(
		repo,
		storage.NewS3Storage(s3.NewFromConfig(awsCfg), os.Getenv("S3_BUCKET")),
		event.NewSNSPublisher(sns.NewFromConfig(awsCfg), os.Getenv("SNS_TOPIC_ARN")),
	)

	h := handler.NewHandler(svc)

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/videos/health", middleware.Metrics("/videos/health", h.Health))
	http.HandleFunc("/videos/upload", middleware.Metrics("/videos/upload", h.Upload))
	http.HandleFunc("/videos/", middleware.Metrics("/videos/", h.Videos))

	log.Printf("Videos service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func mustConnectDB() *sql.DB {
	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=require",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_USERNAME"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
	)
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	if err = db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Println("Connected to PostgreSQL")
	return db
}
