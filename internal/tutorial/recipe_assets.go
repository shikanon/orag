package tutorial

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// prepareVisualAssets persists deterministic PDF assets under the same
// project-private namespace as their verified source archive. The manifest
// snapshot records only logical paths and content digests, never coordinates.
func (s *CloneService) prepareVisualAssets(ctx context.Context, job CloneJob, manifest Manifest) (Manifest, error) {
	template, err := s.catalog.Get(job.TemplateID, job.TemplateVersion)
	if err != nil || template.Modality != ModalityVisualDocument {
		return manifest, err
	}
	archive, found := packObject(manifest, "vidoseek_pdf_document.zip")
	if !found || manifest.VisualRuntime == nil {
		return Manifest{}, ErrRecipeInvalid
	}
	root, err := os.MkdirTemp("", "orag-visual-assets-*")
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: %v", ErrPublicPackTempStorage, err)
	}
	defer os.RemoveAll(root)
	assets, err := ExtractPrivateRecipePDFs(ctx, s.private, PrivateObject{TenantID: job.TenantID, ProjectID: job.ProjectID, JobID: job.ID, Object: VerifiedObject{PackObject: archive}}, filepath.Join(root, "pdfs"), root)
	if err != nil {
		return Manifest{}, err
	}
	manifest.VisualAssets = make([]PackObject, 0, len(assets))
	for _, asset := range assets {
		object := PackObject{Path: "visual/pdf/" + strings.TrimPrefix(asset.Document, "/"), SHA256: asset.SHA256, Bytes: asset.Bytes, ContentType: "application/pdf"}
		if !validObjectPath(object.Path) {
			return Manifest{}, ErrRecipeArchiveUnsafe
		}
		if err := s.private.PutVerified(ctx, PrivateObject{TenantID: job.TenantID, ProjectID: job.ProjectID, JobID: job.ID, Object: VerifiedObject{PackObject: object, TempPath: asset.TempPath}}); err != nil {
			return Manifest{}, err
		}
		manifest.VisualAssets = append(manifest.VisualAssets, object)
	}
	return manifest, nil
}
