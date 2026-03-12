package event

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/hack-fiap233/videos/internal/domain"
)

type mockSNSClient struct {
	publishFunc func(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

func (m *mockSNSClient) Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error) {
	return m.publishFunc(ctx, params, optFns...)
}

func TestPublish_Success(t *testing.T) {
	const topicARN = "arn:aws:sns:us-east-1:123:test"
	var gotARN string
	client := &mockSNSClient{
		publishFunc: func(_ context.Context, p *sns.PublishInput, _ ...func(*sns.Options)) (*sns.PublishOutput, error) {
			gotARN = *p.TopicArn
			return &sns.PublishOutput{}, nil
		},
	}
	pub := NewSNSPublisher(client, topicARN)
	err := pub.Publish(context.Background(), domain.VideoEvent{VideoID: 1, S3Key: "k", Title: "t", UserEmail: "u@e.com"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if gotARN != topicARN {
		t.Errorf("expected topic ARN %s, got %s", topicARN, gotARN)
	}
}

func TestPublish_MessageContainsEvent(t *testing.T) {
	var gotMessage string
	client := &mockSNSClient{
		publishFunc: func(_ context.Context, p *sns.PublishInput, _ ...func(*sns.Options)) (*sns.PublishOutput, error) {
			gotMessage = *p.Message
			return &sns.PublishOutput{}, nil
		},
	}
	pub := NewSNSPublisher(client, "arn:test")
	pub.Publish(context.Background(), domain.VideoEvent{VideoID: 7, Title: "my video"})
	if gotMessage == "" {
		t.Error("expected non-empty message")
	}
}

func TestPublish_Error(t *testing.T) {
	client := &mockSNSClient{
		publishFunc: func(_ context.Context, _ *sns.PublishInput, _ ...func(*sns.Options)) (*sns.PublishOutput, error) {
			return nil, errors.New("sns error")
		},
	}
	pub := NewSNSPublisher(client, "arn:test")
	if err := pub.Publish(context.Background(), domain.VideoEvent{}); err == nil {
		t.Fatal("expected error")
	}
}
