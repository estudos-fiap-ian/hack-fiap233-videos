package event

import (
	"context"
	"encoding/json"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sns"
	"github.com/hack-fiap233/videos/internal/domain"
)

type snsClient interface {
	Publish(ctx context.Context, params *sns.PublishInput, optFns ...func(*sns.Options)) (*sns.PublishOutput, error)
}

type SNSPublisher struct {
	client   snsClient
	topicARN string
}

func NewSNSPublisher(client snsClient, topicARN string) *SNSPublisher {
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
