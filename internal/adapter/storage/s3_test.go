package storage

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type mockS3Client struct {
	putObjectFunc func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	return m.putObjectFunc(ctx, params, optFns...)
}

func TestUpload_Success(t *testing.T) {
	var gotBucket, gotKey string
	client := &mockS3Client{
		putObjectFunc: func(_ context.Context, p *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			gotBucket = *p.Bucket
			gotKey = *p.Key
			return &s3.PutObjectOutput{}, nil
		},
	}
	stor := NewS3Storage(client, "my-bucket")
	if err := stor.Upload(context.Background(), "videos/1/test.mp4", strings.NewReader("content")); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if gotBucket != "my-bucket" {
		t.Errorf("expected bucket=my-bucket, got %s", gotBucket)
	}
	if gotKey != "videos/1/test.mp4" {
		t.Errorf("expected key=videos/1/test.mp4, got %s", gotKey)
	}
}

func TestUpload_Error(t *testing.T) {
	client := &mockS3Client{
		putObjectFunc: func(_ context.Context, _ *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			return nil, errors.New("s3 error")
		},
	}
	stor := NewS3Storage(client, "my-bucket")
	if err := stor.Upload(context.Background(), "key", strings.NewReader("content")); err == nil {
		t.Fatal("expected error")
	}
}
