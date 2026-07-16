package tutorial

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var ErrRecipeArchiveNoPDF = errors.New("tutorial recipe archive has no PDF documents")

// VisualDocumentAsset is a verified, private staging file produced from the
// pinned source archive. Its evidence ID is deterministic and contains no
// storage coordinate.
type VisualDocumentAsset struct {
	Document   string
	EvidenceID string
	TempPath   string
	SHA256     string
	Bytes      int64
}

// ExtractRecipePDFs copies only PDF entries from an already verified archive
// into an empty private staging directory. It never trusts archive paths and
// re-applies all archive limits before writing any output.
func ExtractRecipePDFs(archivePath, outputDir string) ([]VisualDocumentAsset, error) {
	archivePath = strings.TrimSpace(archivePath)
	outputDir = strings.TrimSpace(outputDir)
	if archivePath == "" || outputDir == "" {
		return nil, ErrRecipeArchiveUnsafe
	}
	input, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRecipeArchiveUnsafe, err)
	}
	defer input.Close()
	info, err := input.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 {
		return nil, ErrRecipeArchiveUnsafe
	}
	reader, err := zip.NewReader(input, info.Size())
	if err != nil {
		return nil, ErrRecipeArchiveUnsafe
	}
	if err := ValidateRecipeZIP(reader); err != nil {
		return nil, err
	}
	if err := os.Mkdir(outputDir, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, ErrRecipeArchiveUnsafe
		}
		return nil, fmt.Errorf("%w: %v", ErrPublicPackTempStorage, err)
	}
	assets := make([]VisualDocumentAsset, 0)
	for _, entry := range reader.File {
		if !strings.EqualFold(filepath.Ext(entry.Name), ".pdf") {
			continue
		}
		name := filepath.ToSlash(entry.Name)
		destination := filepath.Join(outputDir, filepath.FromSlash(name))
		rel, err := filepath.Rel(outputDir, destination)
		if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return nil, ErrRecipeArchiveUnsafe
		}
		if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrPublicPackTempStorage, err)
		}
		body, err := entry.Open()
		if err != nil {
			return nil, ErrRecipeArchiveUnsafe
		}
		out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			body.Close()
			return nil, fmt.Errorf("%w: %v", ErrPublicPackTempStorage, err)
		}
		hash := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(out, hash), io.LimitReader(body, int64(entry.UncompressedSize64)+1))
		closeErr := errors.Join(body.Close(), out.Close())
		if copyErr != nil || closeErr != nil || written != int64(entry.UncompressedSize64) {
			_ = os.Remove(destination)
			if copyErr != nil {
				return nil, fmt.Errorf("%w: %v", ErrPublicPackTempStorage, copyErr)
			}
			return nil, ErrRecipeArchiveUnsafe
		}
		assets = append(assets, VisualDocumentAsset{Document: name, EvidenceID: name + "#1", TempPath: destination, SHA256: hex.EncodeToString(hash.Sum(nil)), Bytes: written})
	}
	if len(assets) == 0 {
		return nil, ErrRecipeArchiveNoPDF
	}
	sort.Slice(assets, func(i, j int) bool { return assets[i].Document < assets[j].Document })
	return assets, nil
}
