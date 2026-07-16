package tutorial

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"path"
	"strings"
)

var (
	ErrRecipeSourceSize     = errors.New("tutorial recipe source size is invalid")
	ErrRecipeSourceChecksum = errors.New("tutorial recipe source checksum is invalid")
	ErrRecipeArchiveUnsafe  = errors.New("tutorial recipe archive is unsafe")
)

const (
	maxRecipeArchiveEntries = 10000
	maxRecipeArchiveBytes   = int64(8 << 30)
)

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
