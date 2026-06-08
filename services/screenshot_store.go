package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ScreenshotReadResult carries either raw image bytes or a redirect URL —
// never both. LocalScreenshotStore populates Data; R2ScreenshotStore populates
// RedirectURL (a short-lived presigned S3 URL). Callers must check RedirectURL
// first and issue an HTTP redirect rather than attempting to serve Data.
type ScreenshotReadResult struct {
	ContentType string
	Data        []byte
	RedirectURL string
}

type LocalScreenshotStore struct {
	RootDir string
}

func (s LocalScreenshotStore) StorePNG(_ context.Context, projectKey string, reportID string, eventTime time.Time, png []byte) (string, error) {
	if s.RootDir == "" {
		return "", errors.New("root directory is required")
	}
	key := screenshotObjectKey(projectKey, reportID, eventTime)
	filePath := filepath.Join(s.RootDir, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(filePath), 0o750); err != nil {
		return "", err
	}
	if err := os.WriteFile(filePath, png, 0o640); err != nil {
		return "", err
	}
	return key, nil
}

func (s LocalScreenshotStore) ReadPNG(_ context.Context, key string) (ScreenshotReadResult, error) {
	if s.RootDir == "" {
		return ScreenshotReadResult{}, errors.New("root directory is required")
	}
	if !safeObjectKey(key) {
		return ScreenshotReadResult{}, fmt.Errorf("%w: invalid screenshot object key", ErrBadRequest)
	}
	data, err := os.ReadFile(filepath.Join(s.RootDir, filepath.FromSlash(key)))
	if err != nil {
		return ScreenshotReadResult{}, err
	}
	return ScreenshotReadResult{
		ContentType: "image/png",
		Data:        data,
	}, nil
}

type R2ScreenshotStoreOptions struct {
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	Region          string
}

type R2ScreenshotStore struct {
	bucket string
	client *s3.Client
}

func NewR2ScreenshotStore(ctx context.Context, opts R2ScreenshotStoreOptions) (*R2ScreenshotStore, error) {
	if opts.Endpoint == "" {
		return nil, errors.New("R2 endpoint is required")
	}
	if opts.Bucket == "" {
		return nil, errors.New("R2 bucket is required")
	}
	if opts.AccessKeyID == "" || opts.SecretAccessKey == "" {
		return nil, errors.New("R2 access key ID and secret access key are required")
	}
	region := opts.Region
	if region == "" {
		region = "auto"
	}

	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(opts.AccessKeyID, opts.SecretAccessKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load R2 config: %w", err)
	}

	client := s3.NewFromConfig(cfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(opts.Endpoint)
		options.UsePathStyle = true
	})

	return &R2ScreenshotStore{
		bucket: opts.Bucket,
		client: client,
	}, nil
}

func (s *R2ScreenshotStore) StorePNG(ctx context.Context, projectKey string, reportID string, eventTime time.Time, png []byte) (string, error) {
	if s == nil || s.client == nil {
		return "", errors.New("R2 screenshot store is not configured")
	}
	key := screenshotObjectKey(projectKey, reportID, eventTime)
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader(png),
		ContentType: aws.String("image/png"),
	})
	if err != nil {
		return "", err
	}
	return key, nil
}

// ReadPNG returns a 15-minute presigned S3 URL in ScreenshotReadResult.RedirectURL.
// The image data is never proxied through this service — callers should issue an
// HTTP redirect to RedirectURL, not buffer the content themselves.
func (s *R2ScreenshotStore) ReadPNG(ctx context.Context, key string) (ScreenshotReadResult, error) {
	if s == nil || s.client == nil {
		return ScreenshotReadResult{}, errors.New("R2 screenshot store is not configured")
	}
	if !safeObjectKey(key) {
		return ScreenshotReadResult{}, fmt.Errorf("%w: invalid screenshot object key", ErrBadRequest)
	}
	presigner := s3.NewPresignClient(s.client)
	resp, err := presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(options *s3.PresignOptions) {
		options.Expires = 15 * time.Minute
	})
	if err != nil {
		return ScreenshotReadResult{}, err
	}
	return ScreenshotReadResult{RedirectURL: resp.URL}, nil
}

func screenshotObjectKey(projectKey string, reportID string, eventTime time.Time) string {
	utc := eventTime.UTC()
	return path.Join(
		"bug-reports",
		objectKeyComponent(projectKey, "project"),
		utc.Format("2006"),
		utc.Format("01"),
		utc.Format("02"),
		objectKeyComponent(reportID, "report")+".png",
	)
}

func objectKeyComponent(value string, fallback string) string {
	value = strings.TrimSpace(value)
	var out strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			out.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			out.WriteRune(r)
		case r >= '0' && r <= '9':
			out.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			out.WriteRune(r)
		default:
			out.WriteRune('_')
		}
	}

	sanitized := out.String()
	if sanitized == "" || sanitized == "." || sanitized == ".." {
		return fallback
	}
	return sanitized
}

func safeObjectKey(key string) bool {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		return false
	}
	cleaned := path.Clean(key)
	return cleaned == key &&
		cleaned != "." &&
		!strings.HasPrefix(cleaned, "../") &&
		!strings.Contains(cleaned, "/../")
}
