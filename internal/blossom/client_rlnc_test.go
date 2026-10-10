package blossom

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"anonblossom/internal/rlncbox"
)

func TestClient_DistributedUploadAndInnovativeFetch(t *testing.T) {
	// Création de 3 serveurs Blossom factices
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
	defer srv1.Close()
	srv2 := createMockServer("srv2")
	defer srv2.Close()
	srv3 := createMockServer("srv3")
	defer srv3.Close()

	client := NewClientWithOptions([]string{srv1.URL, srv2.URL, srv3.URL}, http.DefaultClient, "test-agent", ClientOptions{AllowPrivateRelays: true})

	// Création de shards RLNC
	data := []byte("Testing distributed upload across 3 servers with RLNC K=4 N=6")
	k, n := 4, 6
	rlncShards, err := rlncbox.EncodeFile(data, k, n)
	if err != nil {
		t.Fatalf("EncodeFile failed: %v", err)
	}

	rawShards := make([][]byte, n)
	shardHashes := make([]string, n)
	for i, s := range rlncShards {
		raw := rlncbox.SerializeShard(s)
		rawShards[i] = raw
		h := sha256.Sum256(raw)
		shardHashes[i] = hex.EncodeToString(h[:])
	}

	// 1. Upload distribué
	ctx := context.Background()
	usedRelays, err := client.UploadDistributedShards(ctx, rawShards, shardHashes)
	if err != nil {
		t.Fatalf("UploadDistributedShards failed: %v", err)
	}
	if len(usedRelays) < 2 {
		t.Errorf("expected shards to be distributed across at least 2 relays, got %d", len(usedRelays))
	}

	// 2. Fetch innovant en parallèle
	recoveredShards, err := client.FetchInnovativeShards(ctx, shardHashes, usedRelays, k)
	if err != nil {
		t.Fatalf("FetchInnovativeShards failed: %v", err)
	}
	if len(recoveredShards) != k {
		t.Fatalf("expected %d shards, got %d", k, len(recoveredShards))
	}

	// 3. Décodage
	decodedData, err := rlncbox.DecodeFile(recoveredShards, k, len(data))
	if err != nil {
		t.Fatalf("DecodeFile failed: %v", err)
	}
	if string(decodedData) != string(data) {
		t.Fatalf("decoded data mismatch: got %q, want %q", string(decodedData), string(data))
	}
}
