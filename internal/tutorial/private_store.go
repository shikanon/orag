package tutorial

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

var (
	ErrPrivateStoreConfiguration = errors.New("tutorial private output storage is not configured")
	ErrPrivateStoreWrite         = errors.New("tutorial private output storage write failed")
	ErrPrivateStoreRead          = errors.New("tutorial private output storage read failed")
)

type PrivateObject struct {
	TenantID  string
	ProjectID string
	JobID     string
	Object    VerifiedObject
}

// PrivateStore owns user-visible output only. Runtime reads are derived from a
// verified Pack object plus tenant/project/job identity; it never accepts a
// key, URL, or other object-storage detail from an HTTP caller.
type PrivateStore interface {
	PutVerified(context.Context, PrivateObject) error
	HasVerified(context.Context, PrivateObject) (bool, error)
	OpenVerified(context.Context, PrivateObject) (io.ReadCloser, error)
	ReadVerified(context.Context, PrivateObject) ([]byte, error)
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
	destinationDir := s.destinationDir(input)
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

func (s *LocalPrivateStore) ReadVerified(ctx context.Context, input PrivateObject) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || !validPrivateObject(input) {
		return nil, ErrPrivateStoreConfiguration
	}
	filename := filepath.Join(s.destinationDir(input), input.Object.SHA256)
	info, err := os.Stat(filename)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPrivateStoreRead, err)
	}
	if info.Size() != input.Object.Bytes {
		return nil, ErrPrivateStoreRead
	}
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPrivateStoreRead, err)
	}
	defer file.Close()
	return readPrivateObject(file, input.Object.PackObject)
}

// OpenVerified returns a stream for a SHA-addressed private object. Callers
// that consume an archive must verify the byte count and digest while copying
// it; this method deliberately avoids materializing large source data in RAM.
func (s *LocalPrivateStore) OpenVerified(ctx context.Context, input PrivateObject) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || !validPrivateObject(input) {
		return nil, ErrPrivateStoreConfiguration
	}
	file, err := os.Open(filepath.Join(s.destinationDir(input), input.Object.SHA256))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPrivateStoreRead, err)
	}
	return file, nil
}

func (s *LocalPrivateStore) HasVerified(ctx context.Context, input PrivateObject) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if s == nil || !validPrivateObject(input) {
		return false, ErrPrivateStoreConfiguration
	}
	info, err := os.Stat(filepath.Join(s.destinationDir(input), input.Object.SHA256))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrPrivateStoreRead, err)
	}
	return info.Mode().IsRegular() && info.Size() == input.Object.Bytes, nil
}

func (s *LocalPrivateStore) destinationDir(input PrivateObject) string {
	return filepath.Join(s.root, s.prefix, input.TenantID, input.ProjectID, input.JobID)
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
	key := s.objectKey(input)
	if err := s.bucket.PutObjectFromFile(key, input.Object.TempPath, oss.ContentType(input.Object.ContentType)); err != nil {
		return fmt.Errorf("%w: %v", ErrPrivateStoreWrite, err)
	}
	return nil
}

func (s *AliyunPrivateStore) ReadVerified(ctx context.Context, input PrivateObject) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.bucket == nil || !validPrivateObject(input) {
		return nil, ErrPrivateStoreConfiguration
	}
	reader, err := s.bucket.GetObject(s.objectKey(input))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPrivateStoreRead, err)
	}
	defer reader.Close()
	return readPrivateObject(reader, input.Object.PackObject)
}

func (s *AliyunPrivateStore) OpenVerified(ctx context.Context, input PrivateObject) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil || s.bucket == nil || !validPrivateObject(input) {
		return nil, ErrPrivateStoreConfiguration
	}
	reader, err := s.bucket.GetObject(s.objectKey(input))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPrivateStoreRead, err)
	}
	return reader, nil
}

// CopyVerifiedToTemp streams a private object into a new temporary file while
// enforcing its persisted byte count and SHA-256. It is the safe bridge from
// private object storage to file-oriented converters such as ZIP extraction.
func CopyVerifiedToTemp(ctx context.Context, store PrivateStore, input PrivateObject, tempDir string) (string, error) {
	if store == nil || !validPrivateObject(input) {
		return "", ErrPrivateStoreConfiguration
	}
	reader, err := store.OpenVerified(ctx, input)
	if err != nil {
		return "", err
	}
	defer reader.Close()
	file, err := os.CreateTemp(strings.TrimSpace(tempDir), "orag-private-verified-*")
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrPublicPackTempStorage, err)
	}
	filename := file.Name()
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = os.Remove(filename)
		}
	}()
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(reader, input.Object.Bytes+1))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrPrivateStoreRead, err)
	}
	if written != input.Object.Bytes || hex.EncodeToString(hash.Sum(nil)) != input.Object.SHA256 {
		return "", ErrPrivateStoreRead
	}
	if err := file.Sync(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrPublicPackTempStorage, err)
	}
	if err := file.Close(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrPublicPackTempStorage, err)
	}
	keep = true
	return filename, nil
}

func (s *AliyunPrivateStore) HasVerified(ctx context.Context, input PrivateObject) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if s == nil || s.bucket == nil || !validPrivateObject(input) {
		return false, ErrPrivateStoreConfiguration
	}
	meta, err := s.bucket.GetObjectMeta(s.objectKey(input))
	if err != nil {
		var service oss.ServiceError
		if errors.As(err, &service) && (service.Code == "NoSuchKey" || service.StatusCode == 404) {
			return false, nil
		}
		return false, fmt.Errorf("%w: %v", ErrPrivateStoreRead, err)
	}
	return meta.Get("Content-Length") == strconv.FormatInt(input.Object.Bytes, 10), nil
}

func (s *AliyunPrivateStore) objectKey(input PrivateObject) string {
	return strings.Join([]string{s.prefix, input.TenantID, input.ProjectID, input.JobID, input.Object.SHA256}, "/")
}

func validPrivateObject(input PrivateObject) bool {
	return validPrivateComponent(input.TenantID) && validPrivateComponent(input.ProjectID) && validPrivateComponent(input.JobID) && input.Object.Bytes > 0 && sha256Pattern.MatchString(input.Object.SHA256)
}

func readPrivateObject(reader io.Reader, object PackObject) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, object.Bytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPrivateStoreRead, err)
	}
	if int64(len(data)) != object.Bytes {
		return nil, ErrPrivateStoreRead
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != object.SHA256 {
		return nil, ErrPrivateStoreRead
	}
	return data, nil
}
