package app

import (
	"bytes"
	"crypto/rand"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"anonblossom/internal/blossom"
	"anonblossom/internal/config"
	"anonblossom/internal/cryptbox"
	"anonblossom/internal/guard"
	"anonblossom/internal/web"
)

func setupTestServer(t *testing.T) (*Server, map[string]map[string][]byte, func()) {
	storageMap := make(map[string]map[string][]byte)
	var mu sync.Mutex

	createMockServer := func(name string) *httptest.Server {
		mu.Lock()
		storageMap[name] = make(map[string][]byte)
		mu.Unlock()

		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPut && r.URL.Path == "/upload" {
				hash := r.Header.Get("X-SHA-256")
				data, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				mu.Lock()
				storageMap[name][hash] = data
				mu.Unlock()
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"sha256":"` + hash + `"}`))
				return
			}
			if r.Method == http.MethodGet {
				hash := r.URL.Path[1:]
				mu.Lock()
				data, ok := storageMap[name][hash]
				mu.Unlock()
				if !ok {
					http.NotFound(w, r)
					return
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				_, _ = w.Write(data)
				return
			}
			http.NotFound(w, r)
		}))
	}

	srv1 := createMockServer("srv1")
	srv2 := createMockServer("srv2")
	srv3 := createMockServer("srv3")

	cleanup := func() {
		srv1.Close()
		srv2.Close()
		srv3.Close()
	}

	seedURLs := []string{srv1.URL, srv2.URL, srv3.URL}
	blossomClient := blossom.NewClientWithOptions(seedURLs, http.DefaultClient, "test-agent", blossom.ClientOptions{AllowPrivateRelays: true})

	cfg := config.Config{
		ListenAddr:        ":8080",
		BlossomURLs:       seedURLs,
		PublicBaseURL:     "http://localhost:8080",
		MaxUploadBytes:    20 << 20,
		ChunkSize:         1 << 20,
		UploadConcurrency: 2,
	}

	pages, err := web.NewPages()
	if err != nil {
		cleanup()
		t.Fatalf("NewPages failed: %v", err)
	}

	guardConfig := guard.Configure(cfg.MaxUploadBytes+4<<20, cfg.OnionGuardSecure)
	adm, err := guard.New(guardConfig)
	if err != nil {
		cleanup()
		t.Fatalf("guard.New failed: %v", err)
	}

	server := &Server{
		cfg:       cfg,
		storage:   blossomClient,
		pages:     pages,
		uploads:   make(chan struct{}, 2),
		downloads: make(chan struct{}, 2),
		admission: adm,
		cache:     newDownloadCache(10 * time.Minute),
	}

	return server, storageMap, cleanup
}

func TestUploadDownloadSmallFileDistributed(t *testing.T) {
	server, storageMap, cleanup := setupTestServer(t)
	defer cleanup()

	// Petit fichier (100 Ko <= 2 Mo)
	smallData := bytes.Repeat([]byte("Small file data content. "), 4000)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "small.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(smallData)
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	server.handleUpload(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("upload failed with code %d: %s", rec.Code, rec.Body.String())
	}

	// New small-file uploads must store only RLNC shards and an encrypted
	// manifest, never one full monolithic ciphertext blob.
	readyHTML := rec.Body.String()
	if !bytes.Contains([]byte(readyHTML), []byte("small.txt")) {
		t.Fatalf("expected small.txt in response")
	}
	seenShards := make(map[string]bool)
	relayShards := make(map[string]int)
	manifestCount := 0
	for relay, stored := range storageMap {
		for hash, blob := range stored {
			if bytes.HasPrefix(blob, []byte("ABSH")) {
				seenShards[hash] = true
				relayShards[relay]++
			} else {
				manifestCount++
			}
		}
	}
	if len(seenShards) != 6 {
		t.Fatalf("small file stored %d distinct RLNC shards; expected 6", len(seenShards))
	}
	if len(relayShards) < 3 {
		t.Fatalf("small shards were not dispersed across all 3 configured hosts: %v", relayShards)
	}
	if manifestCount < 2 {
		t.Fatalf("small file encrypted manifest not redundantly replicated: %d", manifestCount)
	}
	pathPrefix := "/d/"
	start := bytes.Index([]byte(readyHTML), []byte(pathPrefix))
	if start == -1 {
		t.Fatal("missing small-file share link")
	}
	prefix := readyHTML[start:]
	end := bytes.IndexAny([]byte(prefix), "\" \n<")
	if end < 0 {
		t.Fatal("malformed share link")
	}
	sharePath := prefix[:end]
	dlReq := httptest.NewRequest(http.MethodGet, sharePath+"?download=1", nil)
	dlRec := httptest.NewRecorder()
	server.handleDownload(dlRec, dlReq)
	if dlRec.Code != http.StatusOK || !bytes.Equal(dlRec.Body.Bytes(), smallData) {
		t.Fatalf("small distributed file did not round-trip: status %d, length %d", dlRec.Code, dlRec.Body.Len())
	}
}

func TestUploadDownloadLargeFileRLNC(t *testing.T) {
	server, storageMap, cleanup := setupTestServer(t)
	defer cleanup()

	// Gros fichier (2.5 Mo > 2 Mo threshold)
	fileSize := int(2.5 * 1024 * 1024)
	largeData := make([]byte, fileSize)
	_, _ = rand.Read(largeData)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "large_video.mp4")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(largeData)
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/upload", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	server.handleUpload(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("large upload failed with code %d: %s", rec.Code, rec.Body.String())
	}

	// Vérifier la distribution des shards sur au moins 2 relais distincts
	activeRelaysCount := 0
	for srvName, stored := range storageMap {
		if len(stored) > 0 {
			activeRelaysCount++
			t.Logf("Server %s stored %d blobs", srvName, len(stored))
		}
	}
	if activeRelaysCount < 2 {
		t.Fatalf("expected shards distributed over at least 2 servers, got %d", activeRelaysCount)
	}

	// Extraire l'URL de téléchargement depuis le lien partagé
	// L'URL est de la forme /d/<manifestHash>/<keyText>
	readyHTML := rec.Body.String()
	pathPrefix := "/d/"
	idx := bytes.Index([]byte(readyHTML), []byte(pathPrefix))
	if idx == -1 {
		t.Fatalf("could not find /d/ path in ready HTML")
	}
	sub := readyHTML[idx:]
	endIdx := bytes.IndexAny([]byte(sub), "\" \n<")
	sharePath := sub[:endIdx]
	t.Logf("Share path: %s", sharePath)

	hash, key, ok := cryptbox.ParseDownloadPath(sharePath)
	if !ok {
		t.Fatalf("failed to parse download path: %s", sharePath)
	}

	// 1. Tester la prévisualisation HTML
	viewReq := httptest.NewRequest(http.MethodGet, sharePath, nil)
	viewRec := httptest.NewRecorder()
	server.handleDownload(viewRec, viewReq)
	if viewRec.Code != http.StatusOK {
		t.Fatalf("view failed with code %d: %s", viewRec.Code, viewRec.Body.String())
	}
	viewHTML := viewRec.Body.String()
	if !bytes.Contains([]byte(viewHTML), []byte("large_video.mp4")) {
		t.Fatalf("view HTML did not display actual filename")
	}

	// 2. Tester le téléchargement réel avec décodage RLNC
	dlReq := httptest.NewRequest(http.MethodGet, sharePath+"?download=1", nil)
	dlRec := httptest.NewRecorder()
	server.handleDownload(dlRec, dlReq)
	if dlRec.Code != http.StatusOK {
		t.Fatalf("download failed with code %d: %s", dlRec.Code, dlRec.Body.String())
	}

	downloadedBytes := dlRec.Body.Bytes()
	if len(downloadedBytes) != len(largeData) {
		t.Fatalf("downloaded size mismatch: got %d, want %d", len(downloadedBytes), len(largeData))
	}
	if !bytes.Equal(downloadedBytes, largeData) {
		t.Fatalf("downloaded data does not match original data bit for bit!")
	}
	t.Logf("Successfully uploaded, distributed, fetched K shards, decoded and decrypted %d bytes!", len(downloadedBytes))

	_ = hash
	_ = key
}

func TestSmallNoJSRLNCDoesNotConsumeLargeFileAdmission(t *testing.T) {
	server, stored, cleanup := setupTestServer(t)
	defer cleanup()
	server.rlncUploads = make(chan struct{}, 1)
	// Simulate another process already using the only high-memory RLNC slot.
	server.rlncUploads <- struct{}{}

	postFile := func(data []byte) *httptest.ResponseRecorder {
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		part, err := writer.CreateFormFile("file", "concurrent.txt")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/upload", body)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		rec := httptest.NewRecorder()
		server.handleUpload(rec, req)
		return rec
	}
	small := postFile(bytes.Repeat([]byte("a"), 100))
	if small.Code != http.StatusOK {
		t.Fatalf("small RLNC upload blocked by unrelated heavy upload: %d %s", small.Code, small.Body.String())
	}
	shards := make(map[string]bool)
	for _, replicas := range stored {
		for hash, blob := range replicas {
			if bytes.HasPrefix(blob, []byte("ABSH")) {
				shards[hash] = true
			}
		}
	}
	if len(shards) != 6 {
		t.Fatalf("small uploads still need six distributed shards, got %d", len(shards))
	}
	large := postFile(bytes.Repeat([]byte("b"), 3<<20))
	if large.Code == http.StatusOK {
		t.Fatal("large RLNC upload bypassed occupied high-memory slot")
	}
}
