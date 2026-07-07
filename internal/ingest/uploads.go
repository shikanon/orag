package ingest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/shikanon/orag/internal/platform/id"
)

type UploadStatus string

const (
	UploadStatusUploading UploadStatus = "uploading"
	UploadStatusCompleted UploadStatus = "completed"
	UploadStatusCanceled  UploadStatus = "canceled"
)

var (
	ErrUploadNotFound       = errors.New("upload not found")
	ErrUploadOffsetMismatch = errors.New("upload offset mismatch")
	ErrUploadAlreadyClosed  = errors.New("upload is already closed")
	ErrUploadTooLarge       = errors.New("upload exceeds max size")
	ErrUploadIncomplete     = errors.New("upload is incomplete")
)

type UploadSession struct {
	ID              string       `json:"id"`
	TenantID        string       `json:"tenant_id"`
	KnowledgeBaseID string       `json:"knowledge_base_id"`
	Name            string       `json:"name"`
	SourceURI       string       `json:"source_uri"`
	TotalBytes      int64        `json:"total_bytes,omitempty"`
	ReceivedBytes   int64        `json:"received_bytes"`
	Status          UploadStatus `json:"status"`
	CreatedAt       time.Time    `json:"created_at"`
	UpdatedAt       time.Time    `json:"updated_at"`
	FilePath        string       `json:"-"`
}

type UploadStore interface {
	CreateUpload(ctx context.Context, session UploadSession) (UploadSession, error)
	GetUpload(ctx context.Context, tenantID, id string) (UploadSession, bool, error)
	AppendUpload(ctx context.Context, tenantID, id string, offset int64, chunk []byte, maxBytes int64) (UploadSession, error)
	ReadUpload(ctx context.Context, tenantID, id string) (UploadSession, []byte, error)
	CompleteUpload(ctx context.Context, tenantID, id string) (UploadSession, error)
	CancelUpload(ctx context.Context, tenantID, id string) error
}

type MemoryUploadStore struct {
	mu       sync.Mutex
	dir      string
	sessions map[string]UploadSession
}

func NewMemoryUploadStore() *MemoryUploadStore {
	return &MemoryUploadStore{dir: filepath.Join(os.TempDir(), "orag-uploads"), sessions: map[string]UploadSession{}}
}

func (s *MemoryUploadStore) CreateUpload(_ context.Context, session UploadSession) (UploadSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return UploadSession{}, err
	}
	now := time.Now().UTC()
	if session.ID == "" {
		session.ID = id.New("upl")
	}
	session.Status = UploadStatusUploading
	session.CreatedAt = now
	session.UpdatedAt = now
	session.FilePath = filepath.Join(s.dir, session.ID+".part")
	file, err := os.OpenFile(session.FilePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return UploadSession{}, err
	}
	if err := file.Close(); err != nil {
		return UploadSession{}, err
	}
	s.sessions[session.ID] = session
	return session, nil
}

func (s *MemoryUploadStore) GetUpload(_ context.Context, tenantID, id string) (UploadSession, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok || session.TenantID != tenantID || session.Status == UploadStatusCanceled {
		return UploadSession{}, false, nil
	}
	return session, true, nil
}

func (s *MemoryUploadStore) AppendUpload(_ context.Context, tenantID, id string, offset int64, chunk []byte, maxBytes int64) (UploadSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok || session.TenantID != tenantID || session.Status == UploadStatusCanceled {
		return UploadSession{}, ErrUploadNotFound
	}
	if session.Status != UploadStatusUploading {
		return session, ErrUploadAlreadyClosed
	}
	if offset != session.ReceivedBytes {
		return session, ErrUploadOffsetMismatch
	}
	if session.TotalBytes > 0 && session.ReceivedBytes+int64(len(chunk)) > session.TotalBytes {
		return session, ErrUploadTooLarge
	}
	if maxBytes > 0 && session.ReceivedBytes+int64(len(chunk)) > maxBytes {
		return session, ErrUploadTooLarge
	}
	file, err := os.OpenFile(session.FilePath, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return UploadSession{}, err
	}
	written, writeErr := file.Write(chunk)
	closeErr := file.Close()
	if writeErr != nil {
		return UploadSession{}, writeErr
	}
	if closeErr != nil {
		return UploadSession{}, closeErr
	}
	if written != len(chunk) {
		return UploadSession{}, fmt.Errorf("short write: %d of %d bytes", written, len(chunk))
	}
	session.ReceivedBytes += int64(written)
	session.UpdatedAt = time.Now().UTC()
	s.sessions[id] = session
	return session, nil
}

func (s *MemoryUploadStore) ReadUpload(_ context.Context, tenantID, id string) (UploadSession, []byte, error) {
	s.mu.Lock()
	session, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok || session.TenantID != tenantID || session.Status == UploadStatusCanceled {
		return UploadSession{}, nil, ErrUploadNotFound
	}
	if session.Status != UploadStatusUploading {
		return session, nil, ErrUploadAlreadyClosed
	}
	if session.TotalBytes > 0 && session.ReceivedBytes != session.TotalBytes {
		return session, nil, ErrUploadIncomplete
	}
	body, err := os.ReadFile(session.FilePath)
	if err != nil {
		return UploadSession{}, nil, err
	}
	if int64(len(body)) != session.ReceivedBytes {
		return session, nil, fmt.Errorf("upload file size %d does not match received bytes %d", len(body), session.ReceivedBytes)
	}
	return session, body, nil
}

func (s *MemoryUploadStore) CompleteUpload(_ context.Context, tenantID, id string) (UploadSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok || session.TenantID != tenantID || session.Status == UploadStatusCanceled {
		return UploadSession{}, ErrUploadNotFound
	}
	if session.Status == UploadStatusCompleted {
		return session, nil
	}
	session.Status = UploadStatusCompleted
	session.UpdatedAt = time.Now().UTC()
	s.sessions[id] = session
	return session, nil
}

func (s *MemoryUploadStore) CancelUpload(_ context.Context, tenantID, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[id]
	if !ok || session.TenantID != tenantID || session.Status == UploadStatusCanceled {
		return ErrUploadNotFound
	}
	session.Status = UploadStatusCanceled
	session.UpdatedAt = time.Now().UTC()
	s.sessions[id] = session
	return os.Remove(session.FilePath)
}
