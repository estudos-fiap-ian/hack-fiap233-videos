package event

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/hack-fiap233/videos/internal/domain"
)

type SNSPublisher struct {
	client   *sns.Client
	topicARN string
}

func NewSNSPublisher(client *sns.Client, topicARN string) *SNSPublisher {
	return &SNSPublisher{client: client, topicARN: topicARN}
}

func (p *SNSPublisher) Publish(ctx context.Context, event domain.VideoEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = p.client.Publish(ctx, &sns.PublishInput{
		TopicArn: aws.String(p.topicARN),
		Message:  aws.String(string(payload)),
	})
	return err
}
