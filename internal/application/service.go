package application

import (
	"context"
	"fmt"
	"io"
	"log"

	"github.com/hack-fiap233/videos/internal/domain"
)

type VideoService struct {
	repo      domain.VideoRepository
	storage   domain.VideoStorage
	publisher domain.EventPublisher
}

func NewVideoService(repo domain.VideoRepository, storage domain.VideoStorage, publisher domain.EventPublisher) *VideoService {
	return &VideoService{repo: repo, storage: storage, publisher: publisher}
}

func (s *VideoService) Upload(ctx context.Context, title, description string, file io.Reader, filename, userEmail string) (int, error) {
	id, err := s.repo.Save(ctx, title, description, userEmail)
	if err != nil {
		return 0, err
	}

	s3Key := fmt.Sprintf("videos/%d/%s", id, filename)
	if err := s.storage.Upload(ctx, s3Key, file); err != nil {
		return 0, fmt.Errorf("failed to upload to S3: %w", err)
	}

	if err := s.repo.UpdateS3Key(ctx, id, s3Key); err != nil {
		log.Printf("Failed to update s3_key for video %d: %v", id, err)
	}

	event := domain.VideoEvent{VideoID: id, S3Key: s3Key, Title: title, UserEmail: userEmail}
	if err := s.publisher.Publish(ctx, event); err != nil {
		log.Printf("Failed to publish to SNS: %v", err)
	}

	return id, nil
}

func (s *VideoService) GetByID(ctx context.Context, id int) (*domain.Video, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *VideoService) List(ctx context.Context) ([]domain.Video, error) {
	return s.repo.List(ctx)
}

func (s *VideoService) Create(ctx context.Context, title, description string) (*domain.Video, error) {
	id, err := s.repo.Save(ctx, title, description, "")
	if err != nil {
		return nil, err
	}
	return &domain.Video{ID: id, Title: title, Description: description, Status: "pending"}, nil
}

func (s *VideoService) ListByUser(ctx context.Context, userEmail string) ([]domain.Video, error) {
	return s.repo.ListByUser(ctx, userEmail)
}

func (s *VideoService) HealthCheck(ctx context.Context) error {
	return s.repo.Ping(ctx)
}
