package luscious

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestParallelPagesBoundsWorkersAndPreservesOrder(t *testing.T) {
	const workers, lastPage = 4, 16
	var active atomic.Int64
	var peak atomic.Int64
	pages, err := parallelPages(context.Background(), workers, 1, lastPage, func(_ context.Context, page int) (int, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for previous := peak.Load(); current > previous; previous = peak.Load() {
			if peak.CompareAndSwap(previous, current) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		return page, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := peak.Load(); got < 2 || got > workers {
		t.Fatalf("peak workers = %d, want 2 through %d", got, workers)
	}
	if len(pages) != lastPage {
		t.Fatalf("got %d pages, want %d", len(pages), lastPage)
	}
	for i, page := range pages {
		if page != i+1 {
			t.Fatalf("page at index %d = %d, want %d", i, page, i+1)
		}
	}
}

func TestResolveMediaURL(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: "/pictures/album/example.jpg", want: "https://www.luscious.net/pictures/album/example.jpg"},
		{input: "//cdn.example.com/example.jpg", want: "https://cdn.example.com/example.jpg"},
		{input: "https://cdn.example.com/example.jpg", want: "https://cdn.example.com/example.jpg"},
	}
	for _, test := range tests {
		got, err := resolveMediaURL(test.input)
		if err != nil {
			t.Fatalf("resolveMediaURL(%q): %v", test.input, err)
		}
		if got != test.want {
			t.Errorf("resolveMediaURL(%q) = %q, want %q", test.input, got, test.want)
		}
	}
}
