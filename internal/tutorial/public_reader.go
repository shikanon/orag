package tutorial

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

var (
	ErrPublicPackOrigin      = errors.New("tutorial public pack origin is invalid")
	ErrPublicPackResponse    = errors.New("tutorial public pack response is invalid")
	ErrPublicPackSize        = errors.New("tutorial public pack response exceeds declared size")
	ErrPublicPackContentType = errors.New("tutorial public pack content type does not match manifest")
	ErrPublicPackChecksum    = errors.New("tutorial public pack checksum does not match manifest")
	ErrPublicPackTempStorage = errors.New("tutorial public pack temporary storage failed")
)

type PublicPackReader struct {
	baseURL          *url.URL
	maxManifestBytes int64
	maxObjectBytes   int64
	tempDir          string
	client           *http.Client
}

type VerifiedObject struct {
	PackObject
	TempPath string
}

func NewPublicPackReader(baseURL string, maxManifestBytes, maxObjectBytes int64, timeout time.Duration, tempDir string, baseClient *http.Client) (*PublicPackReader, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, ErrPublicPackOrigin
	}
	if maxManifestBytes <= 0 || maxObjectBytes <= 0 || timeout <= 0 {
		return nil, fmt.Errorf("%w: positive limits and timeout are required", ErrPublicPackOrigin)
	}
	if baseClient == nil {
		baseClient = http.DefaultClient
	}
	client := &http.Client{Timeout: timeout, Transport: baseClient.Transport, Jar: nil, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	return &PublicPackReader{
		baseURL: parsed, maxManifestBytes: maxManifestBytes, maxObjectBytes: maxObjectBytes,
		tempDir: strings.TrimSpace(tempDir), client: client,
	}, nil
}

func (r *PublicPackReader) FetchManifest(ctx context.Context, manifestPath string) ([]byte, error) {
	if r == nil || !validManifestPath(manifestPath) {
		return nil, ErrPublicPackOrigin
	}
	response, err := r.get(ctx, manifestPath)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if !isJSONContentType(response.Header.Get("Content-Type")) {
		return nil, ErrPublicPackContentType
	}
	return readBounded(response.Body, response.ContentLength, r.maxManifestBytes)
}

func (r *PublicPackReader) FetchObject(ctx context.Context, manifestPath string, object PackObject) (VerifiedObject, error) {
	if r == nil || !validManifestPath(manifestPath) || !validObjectPath(object.Path) || object.Bytes <= 0 || object.Bytes > r.maxObjectBytes {
		return VerifiedObject{}, ErrPublicPackOrigin
	}
	objectPath := path.Join(path.Dir(manifestPath), object.Path)
	response, err := r.get(ctx, objectPath)
	if err != nil {
		return VerifiedObject{}, err
	}
	defer response.Body.Close()
	if response.ContentLength > r.maxObjectBytes || (response.ContentLength >= 0 && response.ContentLength != object.Bytes) {
		return VerifiedObject{}, ErrPublicPackSize
	}
	actualType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || !strings.EqualFold(actualType, object.ContentType) {
		return VerifiedObject{}, ErrPublicPackContentType
	}

	file, err := os.CreateTemp(r.tempDir, "orag-tutorial-pack-*")
	if err != nil {
		return VerifiedObject{}, fmt.Errorf("%w: %v", ErrPublicPackTempStorage, err)
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
	n, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(response.Body, object.Bytes+1))
	if err != nil {
		return VerifiedObject{}, fmt.Errorf("%w: %v", ErrPublicPackResponse, err)
	}
	if n != object.Bytes {
		return VerifiedObject{}, ErrPublicPackSize
	}
	if got := hex.EncodeToString(hash.Sum(nil)); got != object.SHA256 {
		return VerifiedObject{}, ErrPublicPackChecksum
	}
	if err := file.Sync(); err != nil {
		return VerifiedObject{}, fmt.Errorf("%w: %v", ErrPublicPackTempStorage, err)
	}
	if err := file.Close(); err != nil {
		return VerifiedObject{}, fmt.Errorf("%w: %v", ErrPublicPackTempStorage, err)
	}
	keep = true
	return VerifiedObject{PackObject: object, TempPath: filename}, nil
}

func (r *PublicPackReader) get(ctx context.Context, catalogPath string) (*http.Response, error) {
	target, err := r.resolve(catalogPath)
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPublicPackOrigin, err)
	}
	response, err := r.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPublicPackResponse, err)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, ErrPublicPackResponse
	}
	return response, nil
}

func (r *PublicPackReader) resolve(catalogPath string) (*url.URL, error) {
	if catalogPath == "" || strings.HasPrefix(catalogPath, "/") || strings.Contains(catalogPath, "\\") || !validObjectPath(catalogPath) {
		return nil, ErrPublicPackOrigin
	}
	decoded, err := url.PathUnescape(catalogPath)
	if err != nil || decoded != catalogPath {
		return nil, ErrPublicPackOrigin
	}
	resolved := *r.baseURL
	resolved.Path = path.Join(r.baseURL.Path, catalogPath)
	resolved.RawPath = ""
	resolved.RawQuery = ""
	resolved.Fragment = ""
	if resolved.Scheme != r.baseURL.Scheme || resolved.Host != r.baseURL.Host || resolved.User != nil {
		return nil, ErrPublicPackOrigin
	}
	return &resolved, nil
}

func readBounded(body io.Reader, length, limit int64) ([]byte, error) {
	if length > limit {
		return nil, ErrPublicPackSize
	}
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPublicPackResponse, err)
	}
	if int64(len(data)) > limit || (length >= 0 && int64(len(data)) != length) {
		return nil, ErrPublicPackSize
	}
	return data, nil
}

func isJSONContentType(value string) bool {
	contentType, _, err := mime.ParseMediaType(value)
	return err == nil && (strings.EqualFold(contentType, "application/json") || strings.EqualFold(contentType, "application/manifest+json"))
}

func (object VerifiedObject) Remove() error {
	if strings.TrimSpace(object.TempPath) == "" {
		return nil
	}
	return os.Remove(object.TempPath)
}
