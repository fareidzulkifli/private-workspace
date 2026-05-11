package uploads

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/go-chi/chi/v5"

	"private-workspace/internal/httputil"
	"private-workspace/internal/shared"
)

type Config struct {
	AccessKeyID     string
	SecretAccessKey string
	Endpoint        string
	BucketName      string
	Region          string
}

type Client struct {
	bucket    string
	s3        *s3.Client
	presigner *s3.PresignClient
}

func NewClient(cfg Config) (*Client, error) {
	cfg.AccessKeyID = strings.TrimSpace(cfg.AccessKeyID)
	cfg.SecretAccessKey = strings.TrimSpace(cfg.SecretAccessKey)
	cfg.Endpoint = strings.TrimSpace(cfg.Endpoint)
	cfg.BucketName = strings.TrimSpace(cfg.BucketName)
	cfg.Region = strings.TrimSpace(cfg.Region)
	if cfg.Region == "" {
		cfg.Region = "auto"
	}
	if cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" || cfg.Endpoint == "" || cfg.BucketName == "" {
		return nil, errors.New("R2 credentials, endpoint, and bucket are required")
	}

	awsCfg := aws.Config{
		Region:      cfg.Region,
		Credentials: credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(cfg.Endpoint)
		o.UsePathStyle = true
	})
	return &Client{
		bucket:    cfg.BucketName,
		s3:        client,
		presigner: s3.NewPresignClient(client),
	}, nil
}

func (c *Client) PresignPutObject(ctx context.Context, key string, contentType string, expires time.Duration) (string, error) {
	if c == nil {
		return "", errors.New("R2 client is not configured")
	}
	out, err := c.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(c.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expires
	})
	if err != nil {
		return "", fmt.Errorf("presign upload: %w", err)
	}
	return out.URL, nil
}

func (c *Client) PresignGetObject(ctx context.Context, key string, expires time.Duration) (string, error) {
	if c == nil {
		return "", errors.New("R2 client is not configured")
	}
	out, err := c.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expires
	})
	if err != nil {
		return "", fmt.Errorf("presign download: %w", err)
	}
	return out.URL, nil
}

func (c *Client) DeleteObject(ctx context.Context, key string) error {
	if c == nil {
		return errors.New("R2 client is not configured")
	}
	if _, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	}); err != nil {
		return fmt.Errorf("delete R2 object: %w", err)
	}
	return nil
}

type Presigner interface {
	PresignPutObject(ctx context.Context, key string, contentType string, expires time.Duration) (string, error)
}

type Handler struct {
	client Presigner
}

func NewHandler(client Presigner) *Handler {
	return &Handler{client: client}
}

func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/api/upload/presign", h.Presign)
}

func (h *Handler) Presign(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Filename    string `json:"filename"`
		ContentType string `json:"contentType"`
	}
	if err := httputil.DecodeJSON(r, 1<<20, &req); err != nil {
		httputil.WriteError(w, http.StatusBadRequest, "Invalid input")
		return
	}
	req.Filename = strings.TrimSpace(req.Filename)
	req.ContentType = strings.TrimSpace(req.ContentType)
	if req.Filename == "" || req.ContentType == "" {
		httputil.WriteError(w, http.StatusBadRequest, "Filename and content type are required")
		return
	}
	if h.client == nil {
		httputil.WriteError(w, http.StatusInternalServerError, "R2 client is not configured")
		return
	}
	ext := strings.TrimPrefix(filepath.Ext(req.Filename), ".")
	key := "uploads/" + shared.NewID()
	if ext != "" {
		key += "." + ext
	}
	url, err := h.client.PresignPutObject(r.Context(), key, req.ContentType, time.Hour)
	if err != nil {
		httputil.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	httputil.WriteJSON(w, http.StatusOK, map[string]string{"signedUrl": url, "key": key})
}
