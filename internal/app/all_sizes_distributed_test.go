package app

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEveryTinyFileSizeIsDistributedAndRecoverable(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"one-byte", []byte{0x42}},
		{"small-text", []byte("small encrypted message")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, storage, cleanup := setupTestServer(t)
			defer cleanup()

			body := &bytes.Buffer{}
			writer := multipart.NewWriter(body)
			part, err := writer.CreateFormFile("file", "tiny.bin")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write(tc.data); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}

			req := httptest.NewRequest(http.MethodPost, "/upload", body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			rec := httptest.NewRecorder()
			server.handleUpload(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("upload failed for %d bytes: %d %s", len(tc.data), rec.Code, rec.Body.String())
			}
			unique := make(map[string]bool)
			replicas := make(map[string]int)
			for _, blobs := range storage {
				for hash, data := range blobs {
					if bytes.HasPrefix(data, []byte("ABSH")) {
						unique[hash] = true
						replicas[hash]++
					}
				}
			}
			if len(unique) != 6 {
				t.Fatalf("expected exactly six distinct shards, got %d", len(unique))
			}
			for hash := range unique {
				if replicas[hash] != 2 {
					t.Fatalf("shard %s was stored on %d hosts instead of two", hash, replicas[hash])
				}
			}

			// An entire configured relay can become unavailable without
			// losing any encrypted shard: every shard has another copy.
			for key := range storage["srv1"] {
				delete(storage["srv1"], key)
			}
			sharePath := sharePathFromReadyPage(t, rec.Body.String())
			dlReq := httptest.NewRequest(http.MethodGet, sharePath+"?download=1", nil)
			dlRec := httptest.NewRecorder()
			server.handleDownload(dlRec, dlReq)
			if dlRec.Code != http.StatusOK || !bytes.Equal(dlRec.Body.Bytes(), tc.data) {
				t.Fatalf("failed to recover %d-byte file with one relay lost: status %d, len %d",
					len(tc.data), dlRec.Code, dlRec.Body.Len())
			}
		})
	}
}
