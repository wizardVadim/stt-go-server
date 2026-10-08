package transport

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"stt-service/internal/config"
	"stt-service/internal/core/domains/transcription"
	"stt-service/internal/features/transcription/service"

	"github.com/google/uuid"
)

type jobDTO struct {
	ID              string                  `json:"id"`
	Filename        string                  `json:"filename"`
	SizeBytes       int64                   `json:"size_bytes"`
	DurationSeconds float64                 `json:"duration_seconds"`
	Language        string                  `json:"language"`
	Status          transcription.JobStatus `json:"status"`
	Progress        *float64                `json:"progress"`
	CreatedAt       string                  `json:"created_at"`
	UpdatedAt       string                  `json:"updated_at"`
	Error           *string                 `json:"error"`
}

func dto(j transcription.Job) jobDTO {
	var message *string
	if j.ErrorMessage() != "" {
		v := j.ErrorMessage()
		message = &v
	}
	return jobDTO{j.ID().String(), j.OriginalName(), j.SizeBytes(), j.Duration().Seconds(), "en", j.Status(), j.Progress(), j.CreatedAt().UTC().Format(time.RFC3339Nano), j.UpdatedAt().UTC().Format(time.RFC3339Nano), message}
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func failure(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func respondError(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	switch {
	case errors.Is(err, transcription.ErrJobNotFound):
		failure(w, 404, "job_not_found", "Запись не найдена.")
	case errors.Is(err, service.ErrConflict):
		failure(w, 409, "action_unavailable", "Действие недоступно для этой записи.")
	case errors.Is(err, service.ErrTooLarge) || errors.As(err, &tooLarge):
		failure(w, 413, "file_too_large", "Файл превышает допустимый размер.")
	case errors.Is(err, service.ErrNoSpace):
		failure(w, 507, "insufficient_space", "На сервере недостаточно свободного места.")
	case errors.Is(err, transcription.ErrInvalidDuration):
		failure(w, 422, "invalid_duration", "Длительность должна быть известна и составлять не более 10 минут.")
	case errors.Is(err, service.ErrNoAudio):
		failure(w, 422, "no_audio", "Файл не содержит аудиодорожку.")
	case errors.Is(err, service.ErrInvalidMedia):
		failure(w, 422, "invalid_media", "Не удалось прочитать медиафайл.")
	default:
		slog.Error("API operation failed", "kind", "internal")
		failure(w, 500, "internal_error", "Не удалось выполнить операцию на сервере.")
	}
}

func NewHandler(s *service.JobService, cfg config.Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/jobs", func(w http.ResponseWriter, r *http.Request) {
		jobs, err := s.List(r.Context())
		if err != nil {
			respondError(w, err)
			return
		}
		result := make([]jobDTO, 0, len(jobs))
		for _, j := range jobs {
			result = append(result, dto(j))
		}
		writeJSON(w, 200, map[string]any{"jobs": result})
	})
	mux.HandleFunc("POST /api/jobs", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxUploadBytes+(1<<20))
		defer r.Body.Close()
		reader, err := r.MultipartReader()
		if err != nil {
			failure(w, 400, "invalid_upload", "Ожидается multipart-загрузка файла.")
			return
		}
		var upload *service.Upload
		defer func() {
			if upload != nil {
				if err := upload.Close(); err != nil {
					slog.Error("upload cleanup failed")
				}
			}
		}()
		language := ""
		languageSeen := false
		for {
			part, e := reader.NextPart()
			if errors.Is(e, io.EOF) {
				break
			}
			if e != nil {
				multipartError(w, e)
				return
			}
			switch part.FormName() {
			case "file":
				if upload != nil || part.FileName() == "" {
					failure(w, 400, "invalid_upload", "Передайте один файл.")
					return
				}
				upload, e = s.Stage(r.Context(), part.FileName(), part)
				if e != nil {
					respondError(w, e)
					return
				}
			case "language":
				if languageSeen {
					failure(w, 400, "invalid_language", "Передайте язык один раз.")
					return
				}
				languageSeen = true
				var b []byte
				b, e = io.ReadAll(io.LimitReader(part, 16))
				if e != nil {
					multipartError(w, e)
					return
				}
				language = string(b)
				if language != "en" {
					failure(w, 422, "invalid_language", "Поддерживается только английская речь.")
					return
				}
			default:
				failure(w, 400, "invalid_upload", "Неизвестное поле загрузки.")
				return
			}
			if e = part.Close(); e != nil {
				multipartError(w, e)
				return
			}
		}
		if upload == nil || !languageSeen {
			failure(w, 400, "invalid_upload", "Нужны файл и language=en.")
			return
		}
		job, err := s.Submit(r.Context(), upload)
		if err != nil {
			respondError(w, err)
			return
		}
		writeJSON(w, 202, dto(job))
	})
	mux.HandleFunc("GET /api/jobs/{id}/transcript", func(w http.ResponseWriter, r *http.Request) {
		id, ok := jobID(w, r)
		if !ok {
			return
		}
		result, err := s.Transcript(r.Context(), id)
		if err != nil {
			respondError(w, err)
			return
		}
		writeJSON(w, 200, result)
	})
	mux.HandleFunc("POST /api/jobs/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		id, ok := jobID(w, r)
		if !ok {
			return
		}
		job, err := s.Cancel(r.Context(), id)
		if err != nil {
			respondError(w, err)
			return
		}
		writeJSON(w, 200, dto(job))
	})
	mux.HandleFunc("POST /api/jobs/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		id, ok := jobID(w, r)
		if !ok {
			return
		}
		job, err := s.Retry(r.Context(), id)
		if err != nil {
			respondError(w, err)
			return
		}
		writeJSON(w, 202, dto(job))
	})
	mux.HandleFunc("DELETE /api/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, ok := jobID(w, r)
		if !ok {
			return
		}
		if err := s.Remove(r.Context(), id); err != nil {
			respondError(w, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		failure(w, 404, "not_found", "Маршрут не найден.")
	})
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		failure(w, 404, "not_found", "Маршрут не найден.")
	})
	files := http.FileServer(http.Dir(cfg.WebDir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		for _, part := range strings.Split(r.URL.Path, "/") {
			if strings.HasPrefix(part, ".") {
				http.NotFound(w, r)
				return
			}
		}
		if r.URL.Path == "/" {
			http.ServeFile(w, r, filepath.Join(cfg.WebDir, "index.html"))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != "GET" && r.Method != "HEAD" {
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host {
					failure(w, 403, "origin_rejected", "Недопустимый источник запроса.")
					return
				}
			}
		}
		mux.ServeHTTP(w, r)
	})
}
func jobID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		failure(w, 404, "job_not_found", "Запись не найдена.")
		return "", false
	}
	return id.String(), true
}
func multipartError(w http.ResponseWriter, err error) {
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		respondError(w, err)
		return
	}
	failure(w, 400, "invalid_upload", "Повреждённая или незавершённая загрузка.")
}
