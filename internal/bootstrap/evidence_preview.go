package bootstrap

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const previewPrefix = "/evidence-preview/"

type previewReader interface {
	ReadForExecution(context.Context, string, string, string, string) ([]byte, string, error)
}

type evidencePreview struct {
	key    [32]byte
	reader previewReader
	now    func() time.Time
	next   http.Handler
}

func newEvidencePreview(reader previewReader, next http.Handler) (*evidencePreview, error) {
	p := &evidencePreview{reader: reader, next: next, now: time.Now}
	_, err := rand.Read(p.key[:])

	return p, err
}

func (p *evidencePreview) URL(project, execution, check, evidence string) string {
	if !validPreviewID(project) || !validPreviewID(execution) || !validPreviewID(check) || !validPreviewID(evidence) {
		return ""
	}

	path := previewPrefix + strings.Join([]string{project, execution, check, evidence}, "/")
	expires := strconv.FormatInt(p.now().Add(5*time.Minute).Unix(), 10)

	return path + "?expires=" + expires + "&sig=" + p.signature(path, expires)
}

func (p *evidencePreview) signature(path, expires string) string {
	mac := hmac.New(sha256.New, p.key[:])
	_, _ = mac.Write([]byte(path + "\n" + expires))

	return hex.EncodeToString(mac.Sum(nil))
}

func (p *evidencePreview) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, previewPrefix) {
		p.next.ServeHTTP(w, r)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ids := strings.Split(strings.TrimPrefix(r.URL.Path, previewPrefix), "/")
	if len(ids) != 4 {
		http.NotFound(w, r)
		return
	}

	for _, id := range ids {
		if !validPreviewID(id) {
			http.NotFound(w, r)
			return
		}
	}

	query := r.URL.Query()
	expires := query.Get("expires")

	deadline, err := strconv.ParseInt(expires, 10, 64)
	if err != nil || deadline <= p.now().Unix() || deadline > p.now().Add(5*time.Minute).Unix() ||
		len(query["expires"]) != 1 || len(query["sig"]) != 1 {
		http.NotFound(w, r)
		return
	}

	signature, err := hex.DecodeString(query.Get("sig"))
	if err != nil {
		http.NotFound(w, r)
		return
	}

	expected, _ := hex.DecodeString(p.signature(r.URL.Path, expires))
	if !hmac.Equal(signature, expected) {
		http.NotFound(w, r)
		return
	}

	data, mime, err := p.reader.ReadForExecution(r.Context(), ids[0], ids[1], ids[2], ids[3])
	if err != nil || (mime != "image/png" && mime != "image/jpeg") {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", mime)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	_, _ = w.Write(data)
}

func validPreviewID(id string) bool {
	if id == "" {
		return false
	}

	for _, c := range id {
		if c < '0' || c > '9' {
			if c < 'a' || c > 'z' {
				if c < 'A' || c > 'Z' {
					if c != '_' && c != '-' {
						return false
					}
				}
			}
		}
	}

	return true
}
