package blossom

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestVerifiedStreamRejectsCorruptionAndFallsBack(t *testing.T) {
	payload := bytes.Repeat([]byte("encrypted"), 16*1024)
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("tampered"))
	}))
	defer bad.Close()
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer good.Close()
	client := NewClientWithOptions([]string{bad.URL, good.URL}, http.DefaultClient, "test", ClientOptions{AllowPrivateRelays: true})
	file, err := client.FetchVerifiedFile(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}
	name := file.Name()
	got, err := io.ReadAll(file)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("corrupt fetched data: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(name); err != nil {
		t.Fatal(err)
	}
}

func TestVerifiedStreamRejectsOversize(t *testing.T) {
	payload := bytes.Repeat([]byte("c"), 1025)
	sum := sha256.Sum256(payload)
	hash := hex.EncodeToString(sum[:])
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()
	client := NewClientWithOptions([]string{upstream.URL}, http.DefaultClient, "test", ClientOptions{AllowPrivateRelays: true, MaxBlobBytes: 1024})
	if f, err := client.FetchVerifiedFile(context.Background(), hash); err == nil {
		f.Close()
		os.Remove(f.Name())
		t.Fatal("oversized ciphertext was accepted")
	}
}

func TestIndependentReplicasUploadConcurrently(t *testing.T) {
	data := bytes.Repeat([]byte("encrypted"), 256)
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	gate := make(chan struct{})
	var entered atomic.Int32
	server := func() *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, err := io.ReadAll(r.Body)
			if err != nil || !bytes.Equal(body, data) {
				http.Error(w, "bad body", 400)
				return
			}
			if entered.Add(1) == 2 {
				close(gate)
			}
			select {
			case <-gate:
			case <-r.Context().Done():
				return
			case <-time.After(3 * time.Second):
				http.Error(w, "uploads were serialized", 500)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sha256":"` + hash + `"}`))
		}))
	}
	first, second := server(), server()
	defer first.Close()
	defer second.Close()
	client := NewClientWithOptions([]string{first.URL, second.URL}, http.DefaultClient, "test", ClientOptions{AllowPrivateRelays: true})
	if err := client.Upload(context.Background(), bytes.NewReader(data), int64(len(data)), hash); err != nil {
		t.Fatal(err)
	}
	if entered.Load() != 2 {
		t.Fatal("expected two independent replica uploads")
	}
}
