package database

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestUploadServeAndDeleteImage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "images")
	s, err := NewSQLiteStoreFromDSNWithImageDir(":memory:", root)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	fileData := []byte("hello image bytes")
	record, err := s.UploadImage(ctx, "avatars/user1.txt", bytes.NewReader(fileData), "text/plain")
	if err != nil {
		t.Fatalf("UploadImage: %v", err)
	}
	if record == nil {
		t.Fatalf("expected metadata")
	}
	if record.Path != "avatars/user1.txt" {
		t.Fatalf("expected path avatars/user1.txt, got %q", record.Path)
	}
	if record.Size != int64(len(fileData)) {
		t.Fatalf("expected size %d, got %d", len(fileData), record.Size)
	}

	fullPath := filepath.Join(root, "avatars", "user1.txt")
	diskData, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !bytes.Equal(diskData, fileData) {
		t.Fatalf("disk data mismatch")
	}

	meta, err := s.GetImage(ctx, "avatars/user1.txt")
	if err != nil {
		t.Fatalf("GetImage: %v", err)
	}
	if meta == nil {
		t.Fatalf("expected metadata row")
	}
	list, err := s.ListImages(ctx)
	if err != nil {
		t.Fatalf("ListImages: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 image in list, got %d", len(list))
	}
	if list[0].Path != "avatars/user1.txt" {
		t.Fatalf("expected listed path avatars/user1.txt, got %q", list[0].Path)
	}

	req := httptest.NewRequest(http.MethodGet, "/images/avatars/user1.txt", nil)
	rr := httptest.NewRecorder()
	s.ServeImage(rr, req, "avatars/user1.txt")
	if rr.Code != http.StatusOK {
		t.Fatalf("ServeImage status: expected 200, got %d", rr.Code)
	}
	if !bytes.Equal(rr.Body.Bytes(), fileData) {
		t.Fatalf("ServeImage body mismatch")
	}

	if err := s.DeleteImage(ctx, "avatars/user1.txt"); err != nil {
		t.Fatalf("DeleteImage: %v", err)
	}
	if _, err := os.Stat(fullPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected file deleted, stat err=%v", err)
	}
	afterDelete, err := s.GetImage(ctx, "avatars/user1.txt")
	if err != nil {
		t.Fatalf("GetImage after delete: %v", err)
	}
	if afterDelete != nil {
		t.Fatalf("expected nil metadata after delete")
	}
	list, err = s.ListImages(ctx)
	if err != nil {
		t.Fatalf("ListImages after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty image list after delete, got %d", len(list))
	}
}

func TestImageHandlerServesUploadedFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "images")
	s, err := NewSQLiteStoreFromDSNWithImageDir(":memory:", root)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	_, err = s.UploadImage(context.Background(), "sample.bin", bytes.NewReader([]byte("payload")), "")
	if err != nil {
		t.Fatalf("UploadImage: %v", err)
	}

	handler, err := s.ImageHandler("/images/")
	if err != nil {
		t.Fatalf("ImageHandler: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/images/sample.bin", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("handler status: expected 200, got %d", rr.Code)
	}
	if rr.Body.String() != "payload" {
		t.Fatalf("handler body mismatch")
	}
}

func TestUploadImageRejectsTraversalPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "images")
	s, err := NewSQLiteStoreFromDSNWithImageDir(":memory:", root)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	_, err = s.UploadImage(context.Background(), "../../etc/passwd", bytes.NewReader([]byte("x")), "text/plain")
	if !errors.Is(err, ErrInvalidImagePath) {
		t.Fatalf("expected ErrInvalidImagePath, got %v", err)
	}

	_, err = s.UploadImage(context.Background(), "/tmp/evil.txt", bytes.NewReader([]byte("x")), "text/plain")
	if !errors.Is(err, ErrInvalidImagePath) {
		t.Fatalf("expected ErrInvalidImagePath for absolute path, got %v", err)
	}
}
