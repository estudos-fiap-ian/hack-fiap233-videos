package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	_ "github.com/lib/pq"
)

var (
	db          *sql.DB
	s3Client    *s3.Client
	snsClient   *sns.Client
	s3Bucket    string
	snsTopicARN string
)

type Video struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Status      string `json:"status"`
	S3Key       string `json:"s3_key,omitempty"`
	ZipS3Key    string `json:"zip_s3_key,omitempty"`
}

type VideoEvent struct {
	VideoID int    `json:"video_id"`
	S3Key   string `json:"s3_key"`
	Title   string `json:"title"`
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	s3Bucket = os.Getenv("S3_BUCKET")
	snsTopicARN = os.Getenv("SNS_TOPIC_ARN")

	initDB()
	createTable()
	initAWS()

	http.HandleFunc("/videos/health", healthHandler)
	http.HandleFunc("/videos/upload", uploadHandler)
	http.HandleFunc("/videos/", videosHandler)

	log.Printf("Videos service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func initAWS() {
	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}
	s3Client = s3.NewFromConfig(cfg)
	snsClient = sns.NewFromConfig(cfg)
}

func initDB() {
	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=require",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_USERNAME"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
	)

	var err error
	db, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	if err = db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Println("Connected to PostgreSQL")
}

func createTable() {
	create := `CREATE TABLE IF NOT EXISTS videos (
		id SERIAL PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		s3_key TEXT
	)`
	if _, err := db.Exec(create); err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}

	migrations := []string{
		`ALTER TABLE videos ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending'`,
		`ALTER TABLE videos ADD COLUMN IF NOT EXISTS s3_key TEXT`,
	}
	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			log.Fatalf("Failed to run migration: %v", err)
		}
	}
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := db.Ping(); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy", "db": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "videos", "db": "connected"})
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")

	if err := r.ParseMultipartForm(32 << 20); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid multipart form"})
		return
	}

	title := r.FormValue("title")
	description := r.FormValue("description")
	if title == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "title is required"})
		return
	}

	file, header, err := r.FormFile("video")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "video file is required"})
		return
	}
	defer file.Close()

	// Insert in DB first to get the ID
	var videoID int
	err = db.QueryRow(
		"INSERT INTO videos (title, description, status) VALUES ($1, $2, 'pending') RETURNING id",
		title, description,
	).Scan(&videoID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	s3Key := fmt.Sprintf("videos/%d/%s", videoID, header.Filename)

	// Upload to S3
	_, err = s3Client.PutObject(context.Background(), &s3.PutObjectInput{
		Bucket: aws.String(s3Bucket),
		Key:    aws.String(s3Key),
		Body:   file,
	})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "failed to upload to S3"})
		return
	}

	// Update s3_key in DB
	if _, err = db.Exec("UPDATE videos SET s3_key = $1 WHERE id = $2", s3Key, videoID); err != nil {
		log.Printf("Failed to update s3_key for video %d: %v", videoID, err)
	}

	// Publish to SNS
	event := VideoEvent{VideoID: videoID, S3Key: s3Key, Title: title}
	eventJSON, _ := json.Marshal(event)
	_, err = snsClient.Publish(context.Background(), &sns.PublishInput{
		TopicArn: aws.String(snsTopicARN),
		Message:  aws.String(string(eventJSON)),
	})
	if err != nil {
		log.Printf("Failed to publish to SNS: %v", err)
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"job_id":  videoID,
		"status":  "pending",
		"message": "video upload received, processing will start shortly",
	})
}

func videosHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// GET /videos/{id}
	idStr := r.URL.Path[len("/videos/"):]
	if idStr != "" {
		if videoID, err := strconv.Atoi(idStr); err == nil {
			getVideo(w, videoID)
			return
		}
	}

	switch r.Method {
	case http.MethodGet:
		listVideos(w)
	case http.MethodPost:
		createVideo(w, r)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "method not allowed"})
	}
}

func getVideo(w http.ResponseWriter, id int) {
	var v Video
	err := db.QueryRow(
		"SELECT id, title, description, status, COALESCE(s3_key, ''), COALESCE(zip_s3_key, '') FROM videos WHERE id = $1", id,
	).Scan(&v.ID, &v.Title, &v.Description, &v.Status, &v.S3Key, &v.ZipS3Key)
	if err == sql.ErrNoRows {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "video not found"})
		return
	}
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(v)
}

func listVideos(w http.ResponseWriter) {
	rows, err := db.Query("SELECT id, title, description, status, COALESCE(s3_key, ''), COALESCE(zip_s3_key, '') FROM videos ORDER BY id")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	videos := []Video{}
	for rows.Next() {
		var v Video
		if err := rows.Scan(&v.ID, &v.Title, &v.Description, &v.Status, &v.S3Key, &v.ZipS3Key); err != nil {
			continue
		}
		videos = append(videos, v)
	}
	json.NewEncoder(w).Encode(videos)
}

func createVideo(w http.ResponseWriter, r *http.Request) {
	var v Video
	if err := json.NewDecoder(r.Body).Decode(&v); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON"})
		return
	}

	err := db.QueryRow(
		"INSERT INTO videos (title, description, status) VALUES ($1, $2, 'pending') RETURNING id",
		v.Title, v.Description,
	).Scan(&v.ID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	v.Status = "pending"
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(v)
}
