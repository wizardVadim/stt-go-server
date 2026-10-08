package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"stt-service/internal/config"
	"stt-service/internal/features/transcription/repository"
	"stt-service/internal/features/transcription/service"
	"stt-service/internal/storage"
)

func testHandler(t *testing.T) (http.Handler, config.Config) {
	t.Helper()
	root := t.TempDir()
	probe := filepath.Join(root, "ffprobe")
	if err := os.WriteFile(probe, []byte("#!/bin/sh\necho '{\"streams\":[{\"codec_type\":\"audio\"}],\"format\":{\"duration\":\"1\"}}'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{DataDir: filepath.Join(root, "data"), WebDir: filepath.Join(root, "web"), FFprobe: probe, MaxUploadBytes: 16, MinFreeBytes: 1, Retention: 24 * time.Hour}
	db, err := storage.Open(context.Background(), filepath.Join(root, "jobs.sqlite"), "../../../../../migrations")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	s, err := service.NewJobService(repository.NewSQLiteRepository(db), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return NewHandler(s, cfg), cfg
}
func uploadRequest(t *testing.T, language string, size int, duplicate bool) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "lecture.mp4")
	if err != nil {
		t.Fatal(err)
	}
	part.Write([]byte(strings.Repeat("x", size)))
	if duplicate {
		part, _ = writer.CreateFormFile("file", "second")
		part.Write([]byte("x"))
	}
	if language != "" {
		writer.WriteField("language", language)
	}
	writer.Close()
	r := httptest.NewRequest("POST", "/api/jobs", &body)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	return r
}
func TestHTTPUploadAndLifecycle(t *testing.T) {
	h, _ := testHandler(t)
	request := func(method, path string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(method, path, nil))
		return w
	}
	if w := request("GET", "/api/jobs"); w.Code != 200 || !strings.Contains(w.Body.String(), `"jobs":[]`) {
		t.Fatal(w.Code, w.Body.String())
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, uploadRequest(t, "en", 4, false))
	if w.Code != 202 {
		t.Fatal(w.Code, w.Body.String())
	}
	var j jobDTO
	if err := json.Unmarshal(w.Body.Bytes(), &j); err != nil {
		t.Fatal(err)
	}
	if j.Status != "queued" || j.Language != "en" || j.DurationSeconds != 1 || j.Error != nil {
		t.Fatal("bad job DTO")
	}
	if strings.Contains(w.Body.String(), "source_path") {
		t.Fatal("path exposed")
	}
	id := j.ID
	for _, tc := range []struct {
		method, suffix string
		status         int
	}{
		{"GET", "/transcript", 409}, {"DELETE", "", 409}, {"POST", "/cancel", 200}, {"POST", "/retry", 202}, {"POST", "/cancel", 200}, {"DELETE", "", 204}, {"GET", "/transcript", 404}, {"POST", "/retry", 409},
	} {
		w := request(tc.method, "/api/jobs/"+id+tc.suffix)
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d %s", tc.method, tc.suffix, w.Code, w.Body.String())
		}
	}
	if w := request("GET", "/api/jobs"); !strings.Contains(w.Body.String(), `"jobs":[]`) {
		t.Fatal("removed job visible")
	}
	if w := request("GET", "/api/unknown"); w.Code != 404 || !strings.Contains(w.Header().Get("Content-Type"), "application/json") {
		t.Fatal("API fallback returned HTML")
	}
}
func TestRejectedMultipartDoesNotCreateJobs(t *testing.T) {
	for _, tc := range []struct {
		language  string
		size      int
		duplicate bool
		status    int
	}{
		{"ru", 4, false, 422}, {"", 4, false, 400}, {"en", 17, false, 413}, {"en", 4, true, 400}, {"en", 0, false, 422},
	} {
		t.Run(fmt.Sprintf("%s-%d-%v", tc.language, tc.size, tc.duplicate), func(t *testing.T) {
			h, cfg := testHandler(t)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, uploadRequest(t, tc.language, tc.size, tc.duplicate))
			if w.Code != tc.status {
				t.Fatal(w.Code, w.Body.String())
			}
			for _, name := range []string{"tmp", "jobs"} {
				files, err := os.ReadDir(filepath.Join(cfg.DataDir, name))
				if err != nil || len(files) != 0 {
					t.Fatal("leaked upload", name, err)
				}
			}
			w = httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", "/api/jobs", nil))
			if !strings.Contains(w.Body.String(), `"jobs":[]`) {
				t.Fatal("rejected job persisted")
			}
		})
	}
}
