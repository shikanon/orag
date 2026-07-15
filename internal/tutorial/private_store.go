package tutorial

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

var (
	ErrPrivateStoreConfiguration = errors.New("tutorial private output storage is not configured")
	ErrPrivateStoreWrite         = errors.New("tutorial private output storage write failed")
)

type PrivateObject struct {
	TenantID  string
	ProjectID string
	JobID     string
	Object    VerifiedObject
}

// PrivateStore owns user-visible output only. It deliberately has no API for
// fetching public catalog objects and never returns a key or URL to callers.
type PrivateStore interface {
	PutVerified(context.Context, PrivateObject) error
}

type LocalPrivateStore struct {
	root   string
	prefix string
}

type PrivateStoreConfig struct {
	Provider        string
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	AccessKeySecret string
	LocalDirectory  string
	Prefix          string
}

func NewPrivateStore(config PrivateStoreConfig) (PrivateStore, error) {
	switch strings.ToLower(strings.TrimSpace(config.Provider)) {
	case "local":
		return NewLocalPrivateStore(config.LocalDirectory, config.Prefix)
	case "aliyun_oss":
		return NewAliyunPrivateStore(config.Endpoint, config.Bucket, config.AccessKeyID, config.AccessKeySecret, config.Prefix)
	default:
		return nil, ErrPrivateStoreConfiguration
	}
}

func NewLocalPrivateStore(root, prefix string) (*LocalPrivateStore, error) {
	root = strings.TrimSpace(root)
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if root == "" {
		return nil, ErrPrivateStoreConfiguration
	}
	if prefix == "" {
		prefix = "tutorial-experiments"
	}
	if !validPrivateComponent(prefix) {
		return nil, ErrPrivateStoreConfiguration
	}
	return &LocalPrivateStore{root: filepath.Clean(root), prefix: prefix}, nil
}

func (s *LocalPrivateStore) PutVerified(ctx context.Context, input PrivateObject) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || !validPrivateComponent(input.TenantID) || !validPrivateComponent(input.ProjectID) || !validPrivateComponent(input.JobID) || input.Object.TempPath == "" || !sha256Pattern.MatchString(input.Object.SHA256) {
		return ErrPrivateStoreConfiguration
	}
	destinationDir := filepath.Join(s.root, s.prefix, input.TenantID, input.ProjectID, input.JobID)
	if err := os.MkdirAll(destinationDir, 0o700); err != nil {
		return fmt.Errorf("%w: %v", ErrPrivateStoreWrite, err)
	}
	destination := filepath.Join(destinationDir, input.Object.SHA256)
	if existing, err := os.Stat(destination); err == nil {
		if existing.Size() == input.Object.Bytes {
			return nil
		}
		return ErrPrivateStoreWrite
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %v", ErrPrivateStoreWrite, err)
	}
	source, err := os.Open(input.Object.TempPath)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPrivateStoreWrite, err)
	}
	defer source.Close()
	temporary, err := os.CreateTemp(destinationDir, ".pending-*")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPrivateStoreWrite, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	written, err := io.Copy(temporary, source)
	if err != nil || written != input.Object.Bytes {
		temporary.Close()
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPrivateStoreWrite, err)
		}
		return ErrPrivateStoreWrite
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("%w: %v", ErrPrivateStoreWrite, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("%w: %v", ErrPrivateStoreWrite, err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("%w: %v", ErrPrivateStoreWrite, err)
	}
	return nil
}

func validPrivateComponent(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && value == filepath.Clean(value) && !strings.Contains(value, string(filepath.Separator)) && value != "." && value != ".."
}

type AliyunPrivateStore struct {
	bucket *oss.Bucket
	prefix string
}

// NewAliyunPrivateStore configures only the user-owned output bucket. The
// tutorial public origin remains a separate anonymous HTTPS reader.
func NewAliyunPrivateStore(endpoint, bucketName, accessKeyID, accessKeySecret, prefix string) (*AliyunPrivateStore, error) {
	endpoint = strings.TrimSpace(endpoint)
	bucketName = strings.TrimSpace(bucketName)
	accessKeyID = strings.TrimSpace(accessKeyID)
	accessKeySecret = strings.TrimSpace(accessKeySecret)
	prefix = strings.Trim(strings.TrimSpace(prefix), "/")
	if endpoint == "" || bucketName == "" || accessKeyID == "" || accessKeySecret == "" {
		return nil, ErrPrivateStoreConfiguration
	}
	if prefix == "" {
		prefix = "tutorial-experiments"
	}
	if !validPrivateComponent(prefix) {
		return nil, ErrPrivateStoreConfiguration
	}
	client, err := oss.New(endpoint, accessKeyID, accessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPrivateStoreConfiguration, err)
	}
	bucket, err := client.Bucket(bucketName)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPrivateStoreConfiguration, err)
	}
	return &AliyunPrivateStore{bucket: bucket, prefix: prefix}, nil
}

func (s *AliyunPrivateStore) PutVerified(ctx context.Context, input PrivateObject) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.bucket == nil || !validPrivateComponent(input.TenantID) || !validPrivateComponent(input.ProjectID) || !validPrivateComponent(input.JobID) || input.Object.TempPath == "" || !sha256Pattern.MatchString(input.Object.SHA256) {
		return ErrPrivateStoreConfiguration
	}
	if info, err := os.Stat(input.Object.TempPath); err != nil || info.Size() != input.Object.Bytes {
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPrivateStoreWrite, err)
		}
		return ErrPrivateStoreWrite
	}
	key := strings.Join([]string{s.prefix, input.TenantID, input.ProjectID, input.JobID, input.Object.SHA256}, "/")
	if err := s.bucket.PutObjectFromFile(key, input.Object.TempPath, oss.ContentType(input.Object.ContentType)); err != nil {
		return fmt.Errorf("%w: %v", ErrPrivateStoreWrite, err)
	}
	return nil
}
