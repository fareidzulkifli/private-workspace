package backup

import (
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	_ "modernc.org/sqlite"
)

const sqliteDriver = "sqlite"

type Options struct {
	SQLitePath string
	BackupDir  string
	Tier       string
	Now        time.Time
	R2         R2Options
}

type R2Options struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	Region          string
	Prefix          string
}

type Report struct {
	StartedAt   time.Time
	CompletedAt time.Time
	SQLitePath  string
	RawPath     string
	GzipPath    string
	ObjectKey   string
	Uploaded    bool
	Integrity   string
}

func R2OptionsFromEnv() R2Options {
	return R2Options{
		Endpoint:        Env("R2_ENDPOINT", ""),
		AccessKeyID:     Env("R2_ACCESS_KEY_ID", ""),
		SecretAccessKey: Env("R2_SECRET_ACCESS_KEY", ""),
		Bucket:          Env("R2_BUCKET_NAME", ""),
		Region:          Env("R2_REGION", "auto"),
		Prefix:          strings.Trim(Env("R2_BACKUP_PREFIX", "backups/private-workspace"), "/"),
	}
}

func (o R2Options) Complete() bool {
	return o.Endpoint != "" && o.AccessKeyID != "" && o.SecretAccessKey != "" && o.Bucket != ""
}

func Run(ctx context.Context, opts Options) (*Report, error) {
	if opts.SQLitePath == "" {
		return nil, errors.New("sqlite path is required")
	}
	if _, err := os.Stat(opts.SQLitePath); err != nil {
		return nil, fmt.Errorf("stat sqlite source: %w", err)
	}
	if opts.BackupDir == "" {
		opts.BackupDir = "backups"
	}
	if opts.Tier == "" {
		opts.Tier = "hourly"
	}
	if opts.Now.IsZero() {
		opts.Now = time.Now().UTC()
	}
	opts.Tier = strings.ToLower(opts.Tier)
	if opts.Tier != "hourly" && opts.Tier != "daily" && opts.Tier != "monthly" {
		return nil, fmt.Errorf("backup tier must be hourly, daily, or monthly")
	}

	startedAt := time.Now().UTC()
	db, err := openSQLite(ctx, opts.SQLitePath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	integrity, err := integrityCheck(ctx, db)
	if err != nil {
		return nil, err
	}
	if integrity != "ok" {
		return nil, fmt.Errorf("source integrity check failed: %s", integrity)
	}

	tierDir := filepath.Join(opts.BackupDir, opts.Tier)
	if err := os.MkdirAll(tierDir, 0o750); err != nil {
		return nil, fmt.Errorf("create backup directory: %w", err)
	}
	base := backupBaseName(opts.Tier, opts.Now)
	rawPath := filepath.Join(tierDir, base+".db")
	gzipPath := rawPath + ".gz"
	if _, err := os.Stat(rawPath); err == nil {
		return nil, fmt.Errorf("backup already exists: %s", rawPath)
	}
	if _, err := os.Stat(gzipPath); err == nil {
		return nil, fmt.Errorf("backup already exists: %s", gzipPath)
	}

	if _, err := db.ExecContext(ctx, "VACUUM INTO "+quoteSQLiteString(rawPath)); err != nil {
		return nil, fmt.Errorf("vacuum into backup: %w", err)
	}

	backupDB, err := sql.Open(sqliteDriver, rawPath)
	if err != nil {
		return nil, err
	}
	backupIntegrity, err := integrityCheck(ctx, backupDB)
	_ = backupDB.Close()
	if err != nil {
		return nil, err
	}
	if backupIntegrity != "ok" {
		return nil, fmt.Errorf("backup integrity check failed: %s", backupIntegrity)
	}

	if err := gzipFile(rawPath, gzipPath); err != nil {
		return nil, err
	}
	if err := os.Remove(rawPath); err != nil {
		return nil, fmt.Errorf("remove uncompressed backup: %w", err)
	}

	report := &Report{
		StartedAt:   startedAt,
		CompletedAt: time.Now().UTC(),
		SQLitePath:  opts.SQLitePath,
		RawPath:     rawPath,
		GzipPath:    gzipPath,
		Integrity:   backupIntegrity,
	}

	if opts.R2.Complete() {
		objectKey := pathJoin(opts.R2.Prefix, opts.Tier, filepath.Base(gzipPath))
		if err := uploadToR2(ctx, opts.R2, objectKey, gzipPath); err != nil {
			return report, err
		}
		report.ObjectKey = objectKey
		report.Uploaded = true
	}

	return report, nil
}

func (r Report) Markdown() string {
	var b strings.Builder
	fmt.Fprintf(&b, "# SQLite Backup Report\n\n")
	fmt.Fprintf(&b, "- Started at: %s\n", r.StartedAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Completed at: %s\n", r.CompletedAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&b, "- Source SQLite file: `%s`\n", r.SQLitePath)
	fmt.Fprintf(&b, "- Backup file: `%s`\n", r.GzipPath)
	fmt.Fprintf(&b, "- Integrity check: `%s`\n", r.Integrity)
	fmt.Fprintf(&b, "- Uploaded to R2: `%t`\n", r.Uploaded)
	if r.ObjectKey != "" {
		fmt.Fprintf(&b, "- R2 object key: `%s`\n", r.ObjectKey)
	}
	return b.String()
}

func openSQLite(ctx context.Context, path string) (*sql.DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("sqlite path is required")
	}
	db, err := sql.Open(sqliteDriver, path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	configureCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA wal_autocheckpoint = 1000",
	}
	for _, stmt := range pragmas {
		if _, err := db.ExecContext(configureCtx, stmt); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("set %s: %w", stmt, err)
		}
	}
	return db, nil
}

func integrityCheck(ctx context.Context, db *sql.DB) (string, error) {
	var integrity string
	if err := db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return "", fmt.Errorf("integrity check: %w", err)
	}
	return integrity, nil
}

func backupBaseName(tier string, now time.Time) string {
	now = now.UTC()
	switch tier {
	case "daily":
		return now.Format("2006-01-02")
	case "monthly":
		return now.Format("2006-01")
	default:
		return now.Format("2006-01-02T150405Z")
	}
}

func gzipFile(sourcePath, targetPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open raw backup: %w", err)
	}
	defer source.Close()

	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o640)
	if err != nil {
		return fmt.Errorf("create gzip backup: %w", err)
	}
	defer target.Close()

	writer := gzip.NewWriter(target)
	writer.Name = filepath.Base(sourcePath)
	writer.ModTime = time.Now().UTC()
	if _, err := io.Copy(writer, source); err != nil {
		_ = writer.Close()
		return fmt.Errorf("compress backup: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("finish gzip backup: %w", err)
	}
	return nil
}

func uploadToR2(ctx context.Context, opts R2Options, objectKey, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open backup for upload: %w", err)
	}
	defer file.Close()

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(opts.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(opts.AccessKeyID, opts.SecretAccessKey, "")),
	)
	if err != nil {
		return fmt.Errorf("load aws config: %w", err)
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(opts.Endpoint)
		o.UsePathStyle = true
	})

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(opts.Bucket),
		Key:         aws.String(objectKey),
		Body:        file,
		ContentType: aws.String("application/gzip"),
	})
	if err != nil {
		return fmt.Errorf("upload backup to R2: %w", err)
	}
	return nil
}

func pathJoin(parts ...string) string {
	var cleaned []string
	for _, part := range parts {
		part = strings.Trim(part, "/")
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	return strings.Join(cleaned, "/")
}

func quoteSQLiteString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
