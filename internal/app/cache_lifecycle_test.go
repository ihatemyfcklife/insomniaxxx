package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"anonblossom/internal/cryptbox"
)

func TestClosingCacheDuringDecryptRemovesPlaintext(t *testing.T) {
	c := newDownloadCache(time.Millisecond)
	started := make(chan struct{})
	release := make(chan struct{})
	filePath := filepath.Join(t.TempDir(), "decrypted-plaintext")
	result := make(chan error, 1)
	go func() {
		_, _, err := c.GetOrDecrypt(context.Background(), "key", func() (string, cryptbox.FileMeta, error) {
			close(started)
			<-release
			if err := os.WriteFile(filePath, []byte("private"), 0600); err != nil {
				return "", cryptbox.FileMeta{}, err
			}
			return filePath, cryptbox.FileMeta{Name: "test.txt", Size: 7}, nil
		})
		result <- err
	}()
	<-started
	c.Close()
	c.Close() // Shutdown must be idempotent.
	close(release)
	select {
	case err := <-result:
		if err == nil || !strings.Contains(err.Error(), "closed") {
			t.Fatalf("expected closed cache error, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cache decrypt did not terminate")
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("plaintext cache file survived shutdown: %v", err)
	}
}
