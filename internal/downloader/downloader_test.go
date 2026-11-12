package downloader

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LewdLinux1337/luscious-downloader/internal/netx"
)

func TestSharedLimiterCapsConcurrentDownloadsAcrossHosts(t *testing.T) {
	const workers = 4
	var active atomic.Int64
	var peak atomic.Int64
	arrived := make(chan struct{})
	release := make(chan struct{})
	var arrivedOnce, releaseOnce sync.Once
	finishRequests := func() { releaseOnce.Do(func() { close(release) }) }
	defer finishRequests()

	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		for previous := peak.Load(); current > previous; previous = peak.Load() {
			if peak.CompareAndSwap(previous, current) {
				break
			}
		}
		if current >= workers {
			arrivedOnce.Do(func() { close(arrived) })
		}
		<-release
		_, _ = w.Write([]byte("image"))
	})
	firstHost := httptest.NewServer(handler)
	defer firstHost.Close()
	secondHost := httptest.NewServer(handler)
	defer secondHost.Close()

	network := netx.New(5*time.Second, workers, "")
	base := New(network.HTTP, network.Limiter, workers, 0)
	firstFiles := make([]File, workers*2)
	secondFiles := make([]File, workers*2)
	for i := range firstFiles {
		firstFiles[i] = File{ID: fmt.Sprint(i), URL: fmt.Sprintf("%s/%d.bin", firstHost.URL, i)}
		secondFiles[i] = File{ID: fmt.Sprint(i + workers*2), URL: fmt.Sprintf("%s/%d.bin", secondHost.URL, i)}
	}
	firstDir := filepath.Join(t.TempDir(), "first")
	secondDir := filepath.Join(t.TempDir(), "second")
	results := make(chan error, 2)
	go func() { results <- base.WithWorkers(workers).Download(context.Background(), firstFiles, firstDir) }()
	go func() { results <- base.WithWorkers(workers).Download(context.Background(), secondFiles, secondDir) }()

	select {
	case <-arrived:
	case <-time.After(2 * time.Second):
		finishRequests()
		<-results
		<-results
		t.Fatal("downloads did not reach configured concurrency")
	}
	time.Sleep(30 * time.Millisecond)
	if got := peak.Load(); got != workers {
		finishRequests()
		<-results
		<-results
		t.Fatalf("peak requests = %d, want %d", got, workers)
	}
	finishRequests()
	for range 2 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(firstDir, "0.bin")); err != nil {
		t.Fatal(err)
	}
}
