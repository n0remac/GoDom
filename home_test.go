package main

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/n0remac/GoDom/database"
	. "github.com/n0remac/GoDom/websocket"
)

func TestHomeUploadAndDisplayImage(t *testing.T) {
	store, cleanup := newHomeTestStore(t)
	defer cleanup()

	mux := http.NewServeMux()
	Home(mux, NewCommandRegistry(), store)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("image", "cat.png")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	// PNG signature so content-type detection resolves to image/png.
	if _, err := part.Write([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00, 0x00, 0x00, 0x00}); err != nil {
		t.Fatalf("write multipart body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/images/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect status %d, got %d", http.StatusSeeOther, rr.Code)
	}

	images, err := store.ListImages(context.Background())
	if err != nil {
		t.Fatalf("ListImages: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("expected one uploaded image, got %d", len(images))
	}

	homeReq := httptest.NewRequest(http.MethodGet, "/", nil)
	homeRR := httptest.NewRecorder()
	mux.ServeHTTP(homeRR, homeReq)
	if homeRR.Code != http.StatusOK {
		t.Fatalf("expected home status 200, got %d", homeRR.Code)
	}

	bodyText := homeRR.Body.String()
	if !strings.Contains(bodyText, "Gallery") {
		t.Fatalf("expected gallery heading in home page")
	}
	if !strings.Contains(bodyText, "/images/"+images[0].Path) {
		t.Fatalf("expected uploaded image URL in home page")
	}
}

func TestHomeRejectsNonImageUpload(t *testing.T) {
	store, cleanup := newHomeTestStore(t)
	defer cleanup()

	mux := http.NewServeMux()
	Home(mux, NewCommandRegistry(), store)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("image", "notes.txt")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := io.WriteString(part, "plain text file"); err != nil {
		t.Fatalf("write multipart body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/images/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect status %d, got %d", http.StatusSeeOther, rr.Code)
	}

	location := rr.Header().Get("Location")
	if !strings.Contains(location, "uploaded+file+is+not+an+image") {
		t.Fatalf("expected non-image error redirect, got %q", location)
	}

	images, err := store.ListImages(context.Background())
	if err != nil {
		t.Fatalf("ListImages: %v", err)
	}
	if len(images) != 0 {
		t.Fatalf("expected no images after non-image upload, got %d", len(images))
	}
}

func newHomeTestStore(t *testing.T) (*database.DocumentStore, func()) {
	t.Helper()

	imageDir := t.TempDir()
	store, err := database.NewSQLiteStoreFromDSNWithImageDir(":memory:", imageDir)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	return store, func() { _ = store.Close() }
}
