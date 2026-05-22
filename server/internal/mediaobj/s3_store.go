package mediaobj

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Config struct {
	Endpoint        string
	Bucket          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	ForcePathStyle  bool
}

type S3Store struct {
	client  *s3.Client
	presign *s3.PresignClient
	bucket  string
}

func NewS3Store(ctx context.Context, config S3Config) (*S3Store, error) {
	if strings.TrimSpace(config.Endpoint) == "" {
		return nil, errors.New("MEDIA_STORAGE_ENDPOINT is required for S3-compatible media object store")
	}
	if strings.TrimSpace(config.Bucket) == "" {
		return nil, errors.New("MEDIA_STORAGE_BUCKET is required for S3-compatible media object store")
	}
	if strings.TrimSpace(config.Region) == "" {
		return nil, errors.New("MEDIA_STORAGE_REGION is required for S3-compatible media object store")
	}
	if strings.TrimSpace(config.AccessKeyID) == "" {
		return nil, errors.New("MEDIA_STORAGE_ACCESS_KEY_ID is required for S3-compatible media object store")
	}
	if strings.TrimSpace(config.SecretAccessKey) == "" {
		return nil, errors.New("MEDIA_STORAGE_SECRET_ACCESS_KEY is required for S3-compatible media object store")
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(strings.TrimSpace(config.Region)),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				strings.TrimSpace(config.AccessKeyID),
				strings.TrimSpace(config.SecretAccessKey),
				"",
			),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("load S3-compatible media object store config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(strings.TrimSpace(config.Endpoint))
		options.UsePathStyle = config.ForcePathStyle
	})
	return &S3Store{
		client:  client,
		presign: s3.NewPresignClient(client),
		bucket:  strings.TrimSpace(config.Bucket),
	}, nil
}

func (s *S3Store) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	output, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get object %q: %w", key, err)
	}
	return output.Body, nil
}

func (s *S3Store) PresignGetObject(ctx context.Context, key string, ttl time.Duration) (string, error) {
	output, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(options *s3.PresignOptions) {
		options.Expires = ttl
	})
	if err != nil {
		return "", fmt.Errorf("presign object %q: %w", key, err)
	}
	return output.URL, nil
}
