package runtime

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestProjectStopCancelsDownload(t *testing.T) {
	started := make(chan struct{})
	released := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(released) }))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	restore := UseDownloadContext(ctx)
	defer restore()
	done := make(chan error, 1)
	go func() {
		done <- downloadFile(downloadSource{URL: server.URL, SHA256: digest([]byte("unused"))}, filepath.Join(t.TempDir(), "download"), "test")
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("download ignored cancellation")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("download did not stop")
	}
	<-released
}
