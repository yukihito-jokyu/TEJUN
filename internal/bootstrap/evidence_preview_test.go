package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type previewReaderFunc func(context.Context, string, string, string, string) ([]byte, string, error)

func (f previewReaderFunc) ReadForExecution(ctx context.Context, p, e, c, v string) ([]byte, string, error) {
	return f(ctx, p, e, c, v)
}

func TestEvidencePreview(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)

	preview, err := newEvidencePreview(
		previewReaderFunc(func(_ context.Context, p, e, c, v string) ([]byte, string, error) {
			if p != "project" || e != "execution" || c != "check:1" || v != "image" {
				return nil, "", errors.New("missing")
			}

			return []byte("png"), "image/png", nil
		}),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) }),
	)
	if err != nil {
		t.Fatal(err)
	}

	preview.now = func() time.Time { return now }

	valid := preview.URL("project", "execution", "check:1", "image")
	if valid == "" || preview.URL("../project", "execution", "check:1", "image") != "" {
		t.Fatal("invalid preview URL")
	}

	tests := []struct {
		name, url, method string
		want              int
	}{
		{"valid", valid, http.MethodGet, http.StatusOK},
		{"static asset", "/index.html", http.MethodGet, http.StatusAccepted},
		{"other project", preview.URL("other", "execution", "check:1", "image"), http.MethodGet, http.StatusNotFound},
		{
			"missing image",
			preview.URL("project", "execution", "check:1", "missing"),
			http.MethodGet,
			http.StatusNotFound,
		},
		{"tampered path", strings.Replace(valid, "/check:1/", "/other/", 1), http.MethodGet, http.StatusNotFound},
		{
			"traversal",
			"/evidence-preview/project/execution/../image" + valid[strings.IndexByte(valid, '?'):],
			http.MethodGet,
			http.StatusNotFound,
		},
		{"wrong method", valid, http.MethodPost, http.StatusMethodNotAllowed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			preview.ServeHTTP(recorder, httptest.NewRequest(tt.method, tt.url, nil))

			if recorder.Code != tt.want {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.want)
			}

			if strings.HasPrefix(tt.url, previewPrefix) &&
				(recorder.Header().Get("Cache-Control") != "no-store" || recorder.Header().Get("X-Content-Type-Options") != "nosniff") {
				t.Fatal("missing security headers")
			}

			if tt.name == "valid" &&
				(recorder.Body.String() != "png" || recorder.Header().Get("Content-Type") != "image/png") {
				t.Fatal("image response mismatch")
			}
		})
	}

	preview.now = func() time.Time { return now.Add(6 * time.Minute) }
	recorder := httptest.NewRecorder()
	preview.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, valid, nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expired status = %d", recorder.Code)
	}
}
