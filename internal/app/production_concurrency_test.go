package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"anonblossom/internal/cryptbox"
)

func TestConcurrentCacheMissesDoNotConsumeRetentionQuota(t *testing.T) {
	c := newDownloadCache(time.Minute)
	defer c.Close()
	const count = 4
	started := make(chan struct{}, count)
	release := make(chan struct{})
	results := make(chan error, count)
	root := t.TempDir()
	for i := 0; i < count; i++ {
		key := string(rune('a' + i))
		go func() {
			f, _, err := c.GetOrDecrypt(context.Background(), key, func() (string, cryptbox.FileMeta, error) {
				started <- struct{}{}
				<-release
				path := filepath.Join(root, key)
				return path, cryptbox.FileMeta{}, os.WriteFile(path, []byte(key), 0600)
			})
			if err == nil {
				var data []byte
				data, err = io.ReadAll(f)
				f.Close()
				if err == nil && string(data) != key {
					err = io.ErrUnexpectedEOF
				}
			}
			results <- err
		}()
	}
	for i := 0; i < count; i++ {
		<-started
	}
	close(release)
	for i := 0; i < count; i++ {
		if err := <-results; err != nil {
			t.Errorf("valid concurrent cache miss failed: %v", err)
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) > maxPlainCacheEntries || c.usedBytes > maxPlainCacheBytes {
		t.Fatal("completed cache exceeds retention budget")
	}
}

func TestUploadOverloadIsRetryableAndRecovers(t *testing.T) {
	a := &Server{uploads: make(chan struct{}, 1), rlncUploads: make(chan struct{}, 1)}
	a.uploads <- struct{}{}
	for _, path := range []string{"/upload", "/upload-raw"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, path, nil)
		if path == "/upload" {
			a.handleUpload(w, r)
		} else {
			a.handleUploadRaw(w, r)
		}
		if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") == "" {
			t.Fatalf("overload response = %d, headers %v", w.Code, w.Header())
		}
	}
	a.releaseUpload()
	if !a.acquireUpload(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/upload", nil)) {
		t.Fatal("upload admission did not recover")
	}
	a.releaseUpload()
	a.rlncUploads <- struct{}{}
	w := httptest.NewRecorder()
	a.handleUploadRLNC(w, httptest.NewRequest(http.MethodPost, "/upload", nil), nil, cryptbox.FileMeta{Size: 3 << 20})
	if w.Code != http.StatusServiceUnavailable || w.Header().Get("Retry-After") == "" {
		t.Fatal("large upload admission must return a retryable overload")
	}
}

func TestRawUploadReusesMemoryAndDiskMultipartWithoutChangingBytes(t *testing.T) {
	for _, size := range []int{64, 2 << 20} {
		t.Run(string(rune('A' + size/(1<<20))), func(t *testing.T) {
			a, stored, cleanup := setupTestServer(t)
			defer cleanup()
			defer a.Close()
			t.Setenv("TMPDIR", t.TempDir())
			payload := bytes.Repeat([]byte("x"), size)
			copy(payload, "ABSH")
			body := new(bytes.Buffer)
			writer := multipart.NewWriter(body)
			part, err := writer.CreateFormFile("file", "synthetic.blob")
			if err != nil {
				t.Fatal(err)
			}
			_, _ = part.Write(payload)
			_ = writer.Close()
			r := httptest.NewRequest(http.MethodPost, "/upload-raw", body)
			r.Header.Set("Content-Type", writer.FormDataContentType())
			w := httptest.NewRecorder()
			a.handleUploadRaw(w, r)
			if w.Code != http.StatusOK {
				t.Fatalf("raw upload failed: %d %s", w.Code, w.Body.String())
			}
			sum := sha256.Sum256(payload)
			hash := hex.EncodeToString(sum[:])
			var result map[string]string
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || result["hash"] != hash {
				t.Fatal("raw upload returned incorrect hash")
			}
			copies := 0
			for _, blobs := range stored {
				if got, ok := blobs[hash]; ok {
					if !bytes.Equal(got, payload) {
						t.Fatal("replicated multipart bytes differ")
					}
					copies++
				}
			}
			if copies < 2 {
				t.Fatal("raw upload was not replicated")
			}
			entries, err := os.ReadDir(os.Getenv("TMPDIR"))
			if err != nil || len(entries) != 0 {
				t.Fatalf("temporary files leaked: %v %v", entries, err)
			}
		})
	}
}
