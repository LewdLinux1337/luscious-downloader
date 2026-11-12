package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/LewdLinux1337/luscious-downloader/internal/netx"
)

type File struct {
	ID  string
	URL string
}

type Downloader struct {
	client  *http.Client
	limiter *netx.Limiter
	workers int
	retries int
}

func New(client *http.Client, limiter *netx.Limiter, workers, retries int) *Downloader {
	if workers < 1 {
		workers = 1
	}
	if retries < 0 {
		retries = 0
	}
	return &Downloader{
		client:  client,
		limiter: limiter,
		workers: workers,
		retries: retries,
	}
}

func (d *Downloader) WithWorkers(workers int) *Downloader {
	if workers < 1 {
		workers = 1
	}
	return &Downloader{client: d.client, limiter: d.limiter, workers: workers, retries: d.retries}
}

func (d *Downloader) Workers() int {
	return d.workers
}

func (d *Downloader) Download(ctx context.Context, files []File, directory string) error {
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("album returned no downloadable file URLs")
	}
	jobs := make(chan File)
	errs := make(chan error, len(files))
	workers := d.workers
	if workers > len(files) {
		workers = len(files)
	}
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range jobs {
				if err := d.downloadOne(ctx, file, directory); err != nil {
					errs <- err
				}
			}
		}()
	}
	for _, file := range files {
		jobs <- file
	}
	close(jobs)
	wg.Wait()
	close(errs)

	var failures []error
	for err := range errs {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func (d *Downloader) downloadOne(ctx context.Context, file File, directory string) error {
	fileURL := strings.TrimSpace(file.URL)
	if strings.HasPrefix(fileURL, "//") {
		fileURL = "https:" + fileURL
	} else if !strings.Contains(fileURL, "://") {
		fileURL = "https://" + fileURL
	}
	parsed, err := url.Parse(fileURL)
	if err != nil || parsed.Host == "" || (!strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https")) {
		return fmt.Errorf("invalid picture URL %q", file.URL)
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	fileURL = parsed.String()
	name := path.Base(parsed.Path)
	if decoded, err := url.PathUnescape(name); err == nil {
		name = decoded
	}
	name = safeFileName(name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = "picture-" + safeFileName(file.ID)
	}
	target := filepath.Join(directory, name)
	if info, err := os.Stat(target); err == nil {
		if info.IsDir() {
			return fmt.Errorf("destination %s is a directory", target)
		}
		if info.Size() > 0 {
			return nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	for attempt := 0; attempt <= d.retries; attempt++ {
		if attempt > 0 {
			pause := time.Duration(attempt) * 250 * time.Millisecond
			if pause > 3*time.Second {
				pause = 3 * time.Second
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(pause):
			}
		}
		retryable, err := d.downloadAttempt(ctx, fileURL, name, target, directory)
		if err == nil {
			return nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || !retryable {
			return err
		}
		if attempt == d.retries {
			return fmt.Errorf("download %s failed after %d attempts: %w", name, attempt+1, err)
		}
	}
	return nil
}

func (d *Downloader) downloadAttempt(ctx context.Context, fileURL, name, target, directory string) (bool, error) {
	if err := d.limiter.Acquire(ctx); err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		d.limiter.Release()
		return false, fmt.Errorf("%s: %w", name, err)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		d.limiter.Release()
		return true, err
	}
	defer d.limiter.Release()
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
		return true, fmt.Errorf("HTTP %s", resp.Status)
	}
	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0]))
	if mediaType == "text/html" || mediaType == "application/json" {
		return true, fmt.Errorf("server returned %s instead of a media file", mediaType)
	}
	tmp, err := os.CreateTemp(directory, ".lsd-*.part")
	if err != nil {
		return false, err
	}
	written, copyErr := io.Copy(tmp, resp.Body)
	closeErr := tmp.Close()
	if copyErr != nil || closeErr != nil || written == 0 {
		os.Remove(tmp.Name())
		switch {
		case copyErr != nil:
			return true, copyErr
		case closeErr != nil:
			return false, closeErr
		default:
			return true, errors.New("server returned an empty file")
		}
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		os.Remove(tmp.Name())
		return false, err
	}
	if err := os.Rename(tmp.Name(), target); err != nil {
		os.Remove(tmp.Name())
		return false, err
	}
	return false, nil
}

func safeFileName(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = path.Base(name)
	name = strings.TrimSpace(name)
	name = strings.NewReplacer("<", "_", ">", "_", ":", "_", "\"", "_", "/", "_", "\\", "_", "|", "_", "?", "_", "*", "_").Replace(name)
	return strings.Trim(name, ". ")
}
