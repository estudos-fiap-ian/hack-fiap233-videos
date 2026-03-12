package domain

import (
	"context"
	"io"
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
	VideoID   int    `json:"video_id"`
	S3Key     string `json:"s3_key"`
	Title     string `json:"title"`
	UserEmail string `json:"user_email"`
}

type VideoRepository interface {
	Save(ctx context.Context, title, description string) (int, error)
	UpdateS3Key(ctx context.Context, id int, s3Key string) error
	GetByID(ctx context.Context, id int) (*Video, error)
	List(ctx context.Context) ([]Video, error)
	Ping(ctx context.Context) error
}

type VideoStorage interface {
	Upload(ctx context.Context, key string, body io.Reader) error
}

type EventPublisher interface {
	Publish(ctx context.Context, event VideoEvent) error
}
