package packrelease

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/volcengine/ve-tos-golang-sdk/v2/tos"
	"github.com/volcengine/ve-tos-golang-sdk/v2/tos/enum"
)

const DefaultPublicBaseURL = "https://lensrhyme.tos-cn-hongkong.volces.com/tutorial-packs"

var ErrObjectAlreadyExists = errors.New("release object already exists")

type PublishConfig struct {
	ReleaseRoot string
	Endpoint    string
	Bucket      string
	AccessKey   string
	SecretKey   string
}

// Publish uploads a completed local release using create-only writes. It is
// intentionally explicit: callers must opt in after Build has succeeded.
func Publish(ctx context.Context, config PublishConfig) error {
	if strings.TrimSpace(config.ReleaseRoot) == "" || strings.TrimSpace(config.Endpoint) == "" || strings.TrimSpace(config.Bucket) == "" || strings.TrimSpace(config.AccessKey) == "" || strings.TrimSpace(config.SecretKey) == "" {
		return errors.New("release root, endpoint, bucket, and credentials are required")
	}
	client, err := tos.NewClientV2(config.Endpoint, tos.WithRegion("cn-hongkong"), tos.WithCredentials(tos.NewStaticCredentials(config.AccessKey, config.SecretKey)), tos.WithConnectionTimeout(15*time.Second), tos.WithRequestTimeout(2*time.Minute), tos.WithSocketTimeout(2*time.Minute, 2*time.Minute))
	if err != nil {
		return fmt.Errorf("create TOS client: %w", err)
	}
	paths, err := releaseFiles(config.ReleaseRoot)
	if err != nil {
		return err
	}
	prefix, err := releasePrefix(config.ReleaseRoot)
	if err != nil {
		return err
	}
	keys := make([]string, len(paths))
	for index, path := range paths {
		rel, err := filepath.Rel(config.ReleaseRoot, path)
		if err != nil {
			return err
		}
		keys[index] = "tutorial-packs/" + prefix + "/" + filepath.ToSlash(rel)
		if _, err := client.HeadObjectV2(ctx, &tos.HeadObjectV2Input{Bucket: config.Bucket, Key: keys[index]}); err == nil {
			return fmt.Errorf("%w: %s", ErrObjectAlreadyExists, keys[index])
		} else if !isNotFound(err) {
			return fmt.Errorf("check existing %s: %w", keys[index], err)
		}
	}
	for index, path := range paths {
		if err := putNewObject(ctx, client, config.Bucket, keys[index], path); err != nil {
			return err
		}
	}
	return nil
}

func putNewObject(ctx context.Context, client *tos.ClientV2, bucket, key, path string) error {
	if _, err := client.HeadObjectV2(ctx, &tos.HeadObjectV2Input{Bucket: bucket, Key: key}); err == nil {
		return fmt.Errorf("%w: %s", ErrObjectAlreadyExists, key)
	} else if !isNotFound(err) {
		return fmt.Errorf("check existing %s: %w", key, err)
	}
	input, err := os.Open(path)
	if err != nil {
		return err
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil {
		return err
	}
	_, err = client.PutObjectV2(ctx, &tos.PutObjectV2Input{PutObjectBasicInput: tos.PutObjectBasicInput{Bucket: bucket, Key: key, ContentLength: info.Size(), ContentType: contentType(path), CacheControl: "public, max-age=31536000, immutable", ACL: enum.ACLPublicRead, ForbidOverwrite: true}, Content: input})
	if err != nil {
		return fmt.Errorf("upload %s: %w", key, err)
	}
	return nil
}

func isNotFound(err error) bool {
	var server *tos.TosServerError
	return errors.As(err, &server) && server.RequestInfo.StatusCode == http.StatusNotFound
}

// VerifyPublic checks every listed artifact through the anonymous public URL.
// The checksum list is the release contract, so this also verifies data not
// referenced by a runtime manifest (including the complete source archive).
func VerifyPublic(ctx context.Context, releaseRoot, publicBaseURL string) error {
	if publicBaseURL == "" {
		publicBaseURL = DefaultPublicBaseURL
	}
	sums, err := os.ReadFile(filepath.Join(releaseRoot, "SHA256SUMS"))
	if err != nil {
		return err
	}
	base, err := url.Parse(strings.TrimRight(publicBaseURL, "/") + "/")
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	for _, line := range strings.Split(strings.TrimSpace(string(sums)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			return fmt.Errorf("invalid checksum entry %q", line)
		}
		target, err := base.Parse(fields[1])
		if err != nil {
			return err
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err != nil {
			return fmt.Errorf("fetch %s: %w", target, err)
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return fmt.Errorf("fetch %s: status %d", target, response.StatusCode)
		}
		digest := sha256.New()
		_, copyErr := io.Copy(digest, response.Body)
		closeErr := response.Body.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if hex.EncodeToString(digest.Sum(nil)) != fields[0] {
			return fmt.Errorf("public checksum mismatch for %s", fields[1])
		}
	}
	return nil
}

func releaseFiles(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Manifests are uploaded after their referenced objects and the checksum
	// contract, preventing a visible manifest from pointing at absent content.
	sort.Slice(paths, func(i, j int) bool {
		mi, mj := filepath.Base(paths[i]) == "manifest.json", filepath.Base(paths[j]) == "manifest.json"
		if mi != mj {
			return !mi
		}
		return paths[i] < paths[j]
	})
	return paths, nil
}

func releasePrefix(root string) (string, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	version := filepath.Base(absolute)
	template := filepath.Base(filepath.Dir(absolute))
	if version == "." || template == "." || version == "" || template == "" {
		return "", errors.New("release root must end with template/version")
	}
	return template + "/" + version, nil
}

func contentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		return "application/json"
	case ".gz":
		return "application/gzip"
	default:
		return "text/plain"
	}
}
