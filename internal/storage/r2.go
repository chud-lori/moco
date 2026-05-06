package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// R2Config bundles credentials for a Cloudflare R2 bucket.
type R2Config struct {
	AccountID       string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
}

// R2 stores objects in a Cloudflare R2 bucket via the S3-compatible API.
type R2 struct {
	cfg      R2Config
	client   *s3.Client
	uploader *manager.Uploader
}

func NewR2(ctx context.Context, cfg R2Config) (*R2, error) {
	if cfg.AccountID == "" || cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" || cfg.Bucket == "" {
		return nil, errors.New("R2 config requires account ID, access key ID, secret access key, and bucket")
	}
	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID)

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx,
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID, cfg.SecretAccessKey, "")),
		awsconfig.WithRegion("auto"),
	)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(endpoint)
		o.UsePathStyle = true // R2 prefers path-style for S3 compatibility
	})

	return &R2{
		cfg:      cfg,
		client:   client,
		uploader: manager.NewUploader(client),
	}, nil
}

func (r *R2) Put(ctx context.Context, key string, body io.Reader, contentType string, _ int64) error {
	in := &s3.PutObjectInput{
		Bucket: aws.String(r.cfg.Bucket),
		Key:    aws.String(key),
		Body:   body,
	}
	if contentType != "" {
		in.ContentType = aws.String(contentType)
	}
	_, err := r.uploader.Upload(ctx, in)
	return err
}

func (r *R2) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := r.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(r.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, err
	}
	return out.Body, nil
}

func (r *R2) Stat(ctx context.Context, key string) (int64, bool, error) {
	out, err := r.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(r.cfg.Bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var nfe *types.NotFound
		if errors.As(err, &nfe) {
			return 0, false, nil
		}
		var ae smithy.APIError
		if errors.As(err, &ae) && (ae.ErrorCode() == "NoSuchKey" || ae.ErrorCode() == "NotFound") {
			return 0, false, nil
		}
		return 0, false, err
	}
	if out.ContentLength == nil {
		return 0, true, nil
	}
	return *out.ContentLength, true, nil
}

func (r *R2) Delete(ctx context.Context, key string) error {
	_, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(r.cfg.Bucket),
		Key:    aws.String(key),
	})
	return err
}

func (r *R2) DeletePrefix(ctx context.Context, prefix string) error {
	prefix = strings.TrimSuffix(prefix, "/") + "/"
	paginator := s3.NewListObjectsV2Paginator(r.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(r.cfg.Bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return err
		}
		if len(page.Contents) == 0 {
			continue
		}
		objects := make([]types.ObjectIdentifier, 0, len(page.Contents))
		for _, obj := range page.Contents {
			objects = append(objects, types.ObjectIdentifier{Key: obj.Key})
		}
		_, err = r.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(r.cfg.Bucket),
			Delete: &types.Delete{Objects: objects, Quiet: aws.Bool(true)},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *R2) LocalPath(ctx context.Context, key string) (string, func(), error) {
	rc, err := r.Get(ctx, key)
	if err != nil {
		return "", nil, err
	}
	defer rc.Close()
	ext := filepath.Ext(key)
	tmp, err := os.CreateTemp("", "moco-r2-*"+ext)
	if err != nil {
		return "", nil, err
	}
	tmpPath := tmp.Name()
	if _, err := io.Copy(tmp, rc); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return "", nil, err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return "", nil, err
	}
	cleanup := func() { _ = os.Remove(tmpPath) }
	return tmpPath, cleanup, nil
}
