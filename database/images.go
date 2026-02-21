package database

import (
	"bufio"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
)

var ErrInvalidImagePath = errors.New("invalid image path")

// ImageRecord describes an uploaded image file and its metadata.
type ImageRecord struct {
	Path         string    `json:"path"`
	OriginalName string    `json:"original_name"`
	ContentType  string    `json:"content_type"`
	Size         int64     `json:"size"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ImageDir returns the configured image storage root (default: data/images).
func (s *DocumentStore) ImageDir() string {
	return s.imageDir
}

// ImageHandler returns a static handler that serves files from ImageDir.
// Mount with mux.Handle("/images/", handler) and pass "/images/" as routePrefix.
func (s *DocumentStore) ImageHandler(routePrefix string) (http.Handler, error) {
	if err := s.ensureImageDir(); err != nil {
		return nil, err
	}
	prefix := normalizeRoutePrefix(routePrefix)
	return http.StripPrefix(prefix, http.FileServer(http.Dir(s.imageDir))), nil
}

// ServeImage serves a single file from ImageDir using a relative imagePath.
func (s *DocumentStore) ServeImage(w http.ResponseWriter, r *http.Request, imagePath string) {
	_, absPath, err := s.resolveImagePath(imagePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, absPath)
}

// UploadImage stores a file under imagePath inside ImageDir and upserts metadata in sqlite.
// If contentType is empty, it is auto-detected from the file contents.
func (s *DocumentStore) UploadImage(ctx context.Context, imagePath string, src io.Reader, contentType string) (*ImageRecord, error) {
	if src == nil {
		return nil, errors.New("source reader is required")
	}

	cleanPath, absPath, err := s.resolveImagePath(imagePath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return nil, err
	}

	tmpFile, err := os.CreateTemp(filepath.Dir(absPath), ".upload-*")
	if err != nil {
		return nil, err
	}
	tmpPath := tmpFile.Name()
	keepTemp := false
	defer func() {
		if !keepTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	reader := bufio.NewReader(src)
	sniffBytes, _ := reader.Peek(512)
	size, err := io.Copy(tmpFile, reader)
	if closeErr := tmpFile.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(contentType) == "" {
		if len(sniffBytes) == 0 {
			contentType = "application/octet-stream"
		} else {
			contentType = http.DetectContentType(sniffBytes)
		}
	}

	if st, err := os.Stat(absPath); err == nil {
		if st.IsDir() {
			return nil, fmt.Errorf("%w: %s", ErrInvalidImagePath, imagePath)
		}
		if err := os.Remove(absPath); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	if err := os.Rename(tmpPath, absPath); err != nil {
		return nil, err
	}
	keepTemp = true
	if err := os.Chmod(absPath, 0o644); err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	_, err = s.db.ExecContext(ctx, `INSERT INTO images(path, original_name, content_type, size, created_at, updated_at)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(path) DO UPDATE SET
			original_name = excluded.original_name,
			content_type = excluded.content_type,
			size = excluded.size,
			updated_at = excluded.updated_at`,
		cleanPath, filepath.Base(cleanPath), contentType, size, now, now)
	if err != nil {
		return nil, err
	}

	return s.GetImage(ctx, cleanPath)
}

// UploadMultipartImage is a convenience wrapper for multipart form uploads.
func (s *DocumentStore) UploadMultipartImage(ctx context.Context, file multipart.File, header *multipart.FileHeader) (*ImageRecord, error) {
	if file == nil || header == nil {
		return nil, errors.New("file and header are required")
	}
	contentType := header.Header.Get("Content-Type")
	return s.UploadImage(ctx, header.Filename, file, contentType)
}

// GetImage returns metadata for an uploaded image by relative path.
func (s *DocumentStore) GetImage(ctx context.Context, imagePath string) (*ImageRecord, error) {
	cleanPath, _, err := s.resolveImagePath(imagePath)
	if err != nil {
		return nil, err
	}

	row := s.db.QueryRowContext(ctx, `SELECT path, original_name, content_type, size, created_at, updated_at
		FROM images WHERE path = ?`, cleanPath)
	var rec ImageRecord
	var createdAt, updatedAt int64
	if err := row.Scan(&rec.Path, &rec.OriginalName, &rec.ContentType, &rec.Size, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	rec.CreatedAt = time.Unix(createdAt, 0).UTC()
	rec.UpdatedAt = time.Unix(updatedAt, 0).UTC()
	return &rec, nil
}

// ListImages returns all uploaded image metadata ordered by newest update first.
func (s *DocumentStore) ListImages(ctx context.Context) ([]*ImageRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT path, original_name, content_type, size, created_at, updated_at
		FROM images ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*ImageRecord
	for rows.Next() {
		rec := &ImageRecord{}
		var createdAt, updatedAt int64
		if err := rows.Scan(&rec.Path, &rec.OriginalName, &rec.ContentType, &rec.Size, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		rec.CreatedAt = time.Unix(createdAt, 0).UTC()
		rec.UpdatedAt = time.Unix(updatedAt, 0).UTC()
		out = append(out, rec)
	}
	return out, rows.Err()
}

// DeleteImage removes the file from disk and deletes its metadata row.
func (s *DocumentStore) DeleteImage(ctx context.Context, imagePath string) error {
	cleanPath, absPath, err := s.resolveImagePath(imagePath)
	if err != nil {
		return err
	}
	if err := os.Remove(absPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	_, err = s.db.ExecContext(ctx, "DELETE FROM images WHERE path = ?", cleanPath)
	return err
}

func (s *DocumentStore) ensureImageDir() error {
	if strings.TrimSpace(s.imageDir) == "" {
		s.imageDir = defaultImageDir
	}
	return os.MkdirAll(s.imageDir, 0o755)
}

func (s *DocumentStore) resolveImagePath(imagePath string) (string, string, error) {
	if err := s.ensureImageDir(); err != nil {
		return "", "", err
	}
	cleanPath, err := cleanImagePath(imagePath)
	if err != nil {
		return "", "", err
	}

	rootAbs, err := filepath.Abs(s.imageDir)
	if err != nil {
		return "", "", err
	}
	absPath := filepath.Join(rootAbs, filepath.FromSlash(cleanPath))
	if !isPathWithinRoot(rootAbs, absPath) {
		return "", "", fmt.Errorf("%w: %s", ErrInvalidImagePath, imagePath)
	}
	return cleanPath, absPath, nil
}

func cleanImagePath(imagePath string) (string, error) {
	trimmed := strings.TrimSpace(imagePath)
	if trimmed == "" {
		return "", fmt.Errorf("%w: empty path", ErrInvalidImagePath)
	}
	if filepath.IsAbs(trimmed) || filepath.VolumeName(trimmed) != "" {
		return "", fmt.Errorf("%w: %s", ErrInvalidImagePath, imagePath)
	}

	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	if strings.HasPrefix(normalized, "/") {
		return "", fmt.Errorf("%w: %s", ErrInvalidImagePath, imagePath)
	}

	cleaned := path.Clean(normalized)
	cleaned = strings.TrimPrefix(cleaned, "/")
	if cleaned == "" || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("%w: %s", ErrInvalidImagePath, imagePath)
	}
	return cleaned, nil
}

func isPathWithinRoot(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func normalizeRoutePrefix(routePrefix string) string {
	p := "/" + strings.Trim(routePrefix, "/")
	if p == "/" {
		return "/"
	}
	return p + "/"
}
