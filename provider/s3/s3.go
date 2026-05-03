// Package s3 implements bucketfill.ObjectStorage on AWS S3 (and S3-compatible
// services such as MinIO).
//
// Importing this package for side effects registers it under the name "s3":
//
//	import _ "github.com/n1kola-petrovic/bucketfill/provider/s3"
package s3

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/n1kola-petrovic/bucketfill"
)

func init() {
	bucketfill.RegisterProvider("s3", openFromConfig)
}

// Storage implements bucketfill.ObjectStorage backed by an S3 client.
type Storage struct {
	client *s3.Client
}

// New builds a Storage from an existing S3 client.
func New(client *s3.Client) *Storage {
	return &Storage{client: client}
}

func openFromConfig(cfg *bucketfill.Config) (bucketfill.ObjectStorage, error) {
	ctx := context.Background()
	if cfg.S3 == nil {
		return nil, fmt.Errorf("s3: missing s3 config")
	}

	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.S3.Region),
	}
	if cfg.S3.AccessKeyID != "" && cfg.S3.SecretAccessKey != "" {
		loadOpts = append(loadOpts,
			awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
				cfg.S3.AccessKeyID, cfg.S3.SecretAccessKey, "")))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("s3: load aws config: %w", err)
	}

	clientOpts := []func(*s3.Options){}
	if cfg.S3.Endpoint != "" {
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.S3.Endpoint)
		})
	}
	if cfg.S3.UsePathStyle {
		clientOpts = append(clientOpts, func(o *s3.Options) { o.UsePathStyle = true })
	}

	return New(s3.NewFromConfig(awsCfg, clientOpts...)), nil
}

func (s *Storage) Upload(ctx context.Context, bucket, key string, r io.Reader, _ int64, contentType string) error {
	in := &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   r,
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	if _, err := s.client.PutObject(ctx, in); err != nil {
		return fmt.Errorf("s3: upload %s/%s: %w", bucket, key, err)
	}
	return nil
}

func (s *Storage) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil, fmt.Errorf("s3: %s/%s: %w", bucket, key, os.ErrNotExist)
		}
		return nil, fmt.Errorf("s3: download %s/%s: %w", bucket, key, err)
	}
	return out.Body, nil
}

func (s *Storage) Delete(ctx context.Context, bucket, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nsk *types.NoSuchKey
		if errors.As(err, &nsk) {
			return nil
		}
		return fmt.Errorf("s3: delete %s/%s: %w", bucket, key, err)
	}
	return nil
}

func (s *Storage) Copy(ctx context.Context, bucket, srcKey, dstKey string) error {
	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(bucket),
		Key:        aws.String(dstKey),
		CopySource: aws.String(bucket + "/" + srcKey),
	})
	if err != nil {
		return fmt.Errorf("s3: copy %s -> %s: %w", srcKey, dstKey, err)
	}
	return nil
}

func (s *Storage) List(ctx context.Context, bucket, prefix string) ([]string, error) {
	var keys []string
	var token *string
	for {
		out, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: token,
		})
		if err != nil {
			return nil, fmt.Errorf("s3: list %s: %w", bucket, err)
		}
		for _, obj := range out.Contents {
			if obj.Key != nil {
				keys = append(keys, *obj.Key)
			}
		}
		if out.IsTruncated == nil || !*out.IsTruncated {
			break
		}
		token = out.NextContinuationToken
	}
	return keys, nil
}
