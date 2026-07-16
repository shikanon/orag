package tutorial

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"time"
)

var (
	ErrRecipeSourceSize     = errors.New("tutorial recipe source size is invalid")
	ErrRecipeSourceChecksum = errors.New("tutorial recipe source checksum is invalid")
	ErrRecipeArchiveUnsafe  = errors.New("tutorial recipe archive is unsafe")
	ErrRecipeSourceOrigin   = errors.New("tutorial recipe source origin is invalid")
	ErrRecipeSourceResponse = errors.New("tutorial recipe source response is invalid")
)

const (
	maxRecipeArchiveEntries = 10000
	maxRecipeArchiveBytes   = int64(8 << 30)
)

// RecipeSourceReader fetches only the pinned ViDoSeek source declaration.
// It accepts redirects solely to Hugging Face's HTTPS delivery hosts, never a
// URL supplied by a catalog, recipe, or API caller.
type RecipeSourceReader struct {
	tempDir string
	client  *http.Client
}

func NewRecipeSourceReader(timeout time.Duration, tempDir string, baseClient *http.Client) (*RecipeSourceReader, error) {
	if timeout <= 0 {
		return nil, ErrRecipeSourceOrigin
	}
	if baseClient == nil {
		baseClient = http.DefaultClient
	}
	client := &http.Client{Timeout: timeout, Transport: baseClient.Transport, Jar: nil, CheckRedirect: func(request *http.Request, _ []*http.Request) error {
		if !trustedRecipeRedirect(request.URL) {
			return ErrRecipeSourceOrigin
		}
		return nil
	}}
	return &RecipeSourceReader{tempDir: strings.TrimSpace(tempDir), client: client}, nil
}

func (r *RecipeSourceReader) Fetch(ctx context.Context, object RecipeSourceObject) (VerifiedObject, error) {
	if r == nil || !validRecipeObject(object) {
		return VerifiedObject{}, ErrRecipeSourceOrigin
	}
	target, err := recipeSourceURL(object.Path)
	if err != nil {
		return VerifiedObject{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return VerifiedObject{}, fmt.Errorf("%w: %v", ErrRecipeSourceOrigin, err)
	}
	response, err := r.client.Do(request)
	if err != nil {
		return VerifiedObject{}, fmt.Errorf("%w: %v", ErrRecipeSourceResponse, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return VerifiedObject{}, ErrRecipeSourceResponse
	}
	if response.ContentLength >= 0 && response.ContentLength != object.Bytes {
		return VerifiedObject{}, ErrRecipeSourceSize
	}
	file, err := os.CreateTemp(r.tempDir, "orag-tutorial-recipe-*")
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
	if err := VerifyRecipeSource(io.TeeReader(response.Body, file), object); err != nil {
		return VerifiedObject{}, err
	}
	if err := file.Sync(); err != nil {
		return VerifiedObject{}, fmt.Errorf("%w: %v", ErrPublicPackTempStorage, err)
	}
	if err := file.Close(); err != nil {
		return VerifiedObject{}, fmt.Errorf("%w: %v", ErrPublicPackTempStorage, err)
	}
	if object.Path == "vidoseek_pdf_document.zip" {
		if err := validateRecipeArchiveFile(filename, object.Bytes); err != nil {
			return VerifiedObject{}, err
		}
	}
	keep = true
	return VerifiedObject{PackObject: PackObject{Path: object.Path, SHA256: object.SHA256, Bytes: object.Bytes, ContentType: recipeContentType(object.Path)}, TempPath: filename}, nil
}

func validateRecipeArchiveFile(filename string, size int64) error {
	file, err := os.Open(filename)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPublicPackTempStorage, err)
	}
	defer file.Close()
	archive, err := zip.NewReader(file, size)
	if err != nil {
		return ErrRecipeArchiveUnsafe
	}
	return ValidateRecipeZIP(archive)
}

func recipeSourceURL(objectPath string) (*url.URL, error) {
	if !validRecipeObject(RecipeSourceObject{Path: objectPath, SHA256: strings.Repeat("0", 64), Bytes: 1}) {
		return nil, ErrRecipeSourceOrigin
	}
	return url.Parse("https://huggingface.co/datasets/" + ViDoSeekDataset + "/resolve/" + ViDoSeekRevision + "/" + objectPath + "?download=true")
}

func trustedRecipeRedirect(target *url.URL) bool {
	if target == nil || target.Scheme != "https" || target.User != nil {
		return false
	}
	host := strings.ToLower(target.Hostname())
	return host == "huggingface.co" || strings.HasSuffix(host, ".huggingface.co") || host == "hf.co" || strings.HasSuffix(host, ".hf.co")
}

func recipeContentType(objectPath string) string {
	if objectPath == "vidoseek.json" {
		return "application/json"
	}
	return "application/zip"
}

// VerifyRecipeSource streams exactly the declared source bytes and verifies
// their digest before the caller may write them to project-private storage.
func VerifyRecipeSource(reader io.Reader, object RecipeSourceObject) error {
	if !validRecipeObject(object) {
		return ErrRecipeInvalid
	}
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(reader, object.Bytes+1))
	if err != nil {
		return err
	}
	if n != object.Bytes {
		return ErrRecipeSourceSize
	}
	if hex.EncodeToString(hash.Sum(nil)) != object.SHA256 {
		return ErrRecipeSourceChecksum
	}
	return nil
}

// ValidateRecipeZIP rejects unsafe archive paths and archive bombs before a
// visual converter sees any extracted page or document path.
func ValidateRecipeZIP(archive *zip.Reader) error {
	if archive == nil || len(archive.File) == 0 || len(archive.File) > maxRecipeArchiveEntries {
		return ErrRecipeArchiveUnsafe
	}
	var total int64
	seen := make(map[string]struct{}, len(archive.File))
	for _, entry := range archive.File {
		name := entry.Name
		if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || path.Clean(name) != name || name == "." || strings.HasPrefix(name, "../") || entry.FileInfo().Mode()&0o170000 == 0o120000 || entry.UncompressedSize64 > uint64(maxRecipeArchiveBytes) {
			return ErrRecipeArchiveUnsafe
		}
		if _, exists := seen[name]; exists || total > maxRecipeArchiveBytes-int64(entry.UncompressedSize64) {
			return ErrRecipeArchiveUnsafe
		}
		seen[name] = struct{}{}
		total += int64(entry.UncompressedSize64)
	}
	return nil
}
