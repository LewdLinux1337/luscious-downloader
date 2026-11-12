package media

import (
	"archive/zip"
	"bytes"
	"compress/zlib"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func filesBelow(directory string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(directory, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			files = append(files, name)
		}
		return nil
	})
	sort.Strings(files)
	return files, err
}

func CreateCBZ(outputPath, directory string) error {
	files, err := filesBelow(directory)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("album folder has no files")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(outputPath), ".lsd-*.cbz")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	archive := zip.NewWriter(tmp)
	for _, name := range files {
		rel, err := filepath.Rel(directory, name)
		if err != nil {
			archive.Close()
			tmp.Close()
			return err
		}
		entry, err := archive.Create(filepath.ToSlash(rel))
		if err != nil {
			archive.Close()
			tmp.Close()
			return err
		}
		file, err := os.Open(name)
		if err != nil {
			archive.Close()
			tmp.Close()
			return err
		}
		_, copyErr := io.Copy(entry, file)
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil {
			archive.Close()
			tmp.Close()
			if copyErr != nil {
				return copyErr
			}
			return closeErr
		}
	}
	if err := archive.Close(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return replace(outputPath, tmpName)
}

type countingWriter struct {
	writer io.Writer
	count  int64
}

func (w *countingWriter) Write(data []byte) (int, error) {
	n, err := w.writer.Write(data)
	w.count += int64(n)
	return n, err
}

func (w *countingWriter) object(offsets []int64, id int, write func() error) error {
	offsets[id] = w.count
	if _, err := fmt.Fprintf(w, "%d 0 obj\n", id); err != nil {
		return err
	}
	if err := write(); err != nil {
		return err
	}
	_, err := io.WriteString(w, "\nendobj\n")
	return err
}

func CreatePDF(outputPath, directory string) error {
	files, err := filesBelow(directory)
	if err != nil {
		return err
	}
	var images []string
	for _, name := range files {
		switch strings.ToLower(filepath.Ext(name)) {
		case ".jpg", ".jpeg", ".png":
			images = append(images, name)
		}
	}
	if len(images) == 0 {
		return fmt.Errorf("no JPG or PNG images found")
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(outputPath), ".lsd-*.pdf")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	w := &countingWriter{writer: tmp}
	if _, err := w.Write([]byte("%PDF-1.4\n%\xe2\xe3\xcf\xd3\n")); err != nil {
		tmp.Close()
		return err
	}
	objectCount := 2 + len(images)*3
	offsets := make([]int64, objectCount+1)
	if err := w.object(offsets, 1, func() error {
		_, err := io.WriteString(w, "<< /Type /Catalog /Pages 2 0 R >>")
		return err
	}); err != nil {
		tmp.Close()
		return err
	}
	if err := w.object(offsets, 2, func() error {
		if _, err := fmt.Fprintf(w, "<< /Type /Pages /Count %d /Kids [", len(images)); err != nil {
			return err
		}
		for i := range images {
			if _, err := fmt.Fprintf(w, " %d 0 R", 3+i*3); err != nil {
				return err
			}
		}
		_, err := io.WriteString(w, " ] >>")
		return err
	}); err != nil {
		tmp.Close()
		return err
	}

	for i, name := range images {
		file, err := os.Open(name)
		if err != nil {
			tmp.Close()
			return err
		}
		img, _, decodeErr := image.Decode(file)
		closeErr := file.Close()
		if decodeErr != nil {
			tmp.Close()
			return fmt.Errorf("decode %s: %w", filepath.Base(name), decodeErr)
		}
		if closeErr != nil {
			tmp.Close()
			return closeErr
		}
		bounds := img.Bounds()
		width, height := bounds.Dx(), bounds.Dy()
		compressed, err := compressRGB(img)
		if err != nil {
			tmp.Close()
			return err
		}
		pageID, imageID, contentID := 3+i*3, 4+i*3, 5+i*3
		pageWidth, pageHeight := 595.0, 842.0
		scale := math.Min(pageWidth/float64(width), pageHeight/float64(height))
		drawWidth, drawHeight := float64(width)*scale, float64(height)*scale
		x, y := (pageWidth-drawWidth)/2, (pageHeight-drawHeight)/2
		if err := w.object(offsets, pageID, func() error {
			_, err := fmt.Fprintf(w, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Resources << /XObject << /Im0 %d 0 R >> >> /Contents %d 0 R >>", pageWidth, pageHeight, imageID, contentID)
			return err
		}); err != nil {
			tmp.Close()
			return err
		}
		if err := w.object(offsets, imageID, func() error {
			if _, err := fmt.Fprintf(w, "<< /Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode /Length %d >>\nstream\n", width, height, compressed.Len()); err != nil {
				return err
			}
			if _, err := w.Write(compressed.Bytes()); err != nil {
				return err
			}
			_, err := io.WriteString(w, "\nendstream")
			return err
		}); err != nil {
			tmp.Close()
			return err
		}
		content := fmt.Sprintf("q %.2f 0 0 %.2f %.2f %.2f cm /Im0 Do Q\n", drawWidth, drawHeight, x, y)
		if err := w.object(offsets, contentID, func() error {
			if _, err := fmt.Fprintf(w, "<< /Length %d >>\nstream\n", len(content)); err != nil {
				return err
			}
			if _, err := io.WriteString(w, content); err != nil {
				return err
			}
			_, err := io.WriteString(w, "endstream")
			return err
		}); err != nil {
			tmp.Close()
			return err
		}
	}

	xref := w.count
	if _, err := fmt.Fprintf(w, "xref\n0 %d\n0000000000 65535 f \n", objectCount+1); err != nil {
		tmp.Close()
		return err
	}
	for id := 1; id <= objectCount; id++ {
		if _, err := fmt.Fprintf(w, "%010d 00000 n \n", offsets[id]); err != nil {
			tmp.Close()
			return err
		}
	}
	if _, err := fmt.Fprintf(w, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", objectCount+1, xref); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return replace(outputPath, tmpName)
}

func replace(outputPath, tempPath string) error {
	if err := os.Remove(outputPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Rename(tempPath, outputPath)
}

func compressRGB(img image.Image) (*bytes.Buffer, error) {
	bounds := img.Bounds()
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	row := make([]byte, bounds.Dx()*3)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, a := img.At(x, y).RGBA()
			idx := (x - bounds.Min.X) * 3
			row[idx] = byte((r + 0xffff - a) >> 8)
			row[idx+1] = byte((g + 0xffff - a) >> 8)
			row[idx+2] = byte((b + 0xffff - a) >> 8)
		}
		if _, err := w.Write(row); err != nil {
			w.Close()
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return &compressed, nil
}
