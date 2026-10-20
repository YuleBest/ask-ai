package cli

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadScreenshotAttachment(t *testing.T) {
	var status bytes.Buffer
	imageData := image.NewRGBA(image.Rect(0, 0, 2, 1))
	imageData.Set(0, 0, color.RGBA{R: 255, A: 255})
	imageData.Set(1, 0, color.RGBA{B: 255, A: 255})

	attachment, err := loadScreenshotAttachment(&status, func() (image.Image, error) {
		if !strings.Contains(status.String(), "正在截取当前屏幕...") {
			t.Fatalf("capture started before status text: %q", status.String())
		}
		return imageData, nil
	})
	if err != nil {
		t.Fatalf("loadScreenshotAttachment() error = %v", err)
	}
	if attachment.MIMEType != "image/png" {
		t.Fatalf("MIME type = %q, want image/png", attachment.MIMEType)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(attachment.Data))
	if err != nil {
		t.Fatalf("decode screenshot: %v", err)
	}
	if format != "png" || config.Width != 2 || config.Height != 1 {
		t.Fatalf("screenshot = %s %dx%d, want png 2x1", format, config.Width, config.Height)
	}
	if got := status.String(); got != "正在截取当前屏幕...\n" {
		t.Fatalf("status = %q", got)
	}
}

func TestLoadScreenshotAttachmentReportsCaptureError(t *testing.T) {
	var status bytes.Buffer
	if _, err := loadScreenshotAttachment(&status, func() (image.Image, error) {
		return nil, errors.New("capture failed")
	}); err == nil || !strings.Contains(err.Error(), "截取屏幕失败") {
		t.Fatalf("error = %v, want capture failure", err)
	}
}

func TestLoadScreenshotAttachmentRejectsOversizedDimensions(t *testing.T) {
	var status bytes.Buffer
	imageData := image.NewRGBA(image.Rect(0, 0, maxImageDimension+1, 1))
	if _, err := loadScreenshotAttachment(&status, func() (image.Image, error) {
		return imageData, nil
	}); err == nil || !strings.Contains(err.Error(), "截图尺寸无效或过大") {
		t.Fatalf("error = %v, want oversized dimensions", err)
	}
}

func TestCaptureScreenshotTimesOut(t *testing.T) {
	release := make(chan struct{})
	defer close(release)

	if _, err := captureScreenshotWithTimeout(func() (image.Image, error) {
		<-release
		return image.NewRGBA(image.Rect(0, 0, 1, 1)), nil
	}, 20*time.Millisecond); err == nil || !strings.Contains(err.Error(), "截取屏幕超时") {
		t.Fatalf("error = %v, want capture timeout", err)
	}
}

func TestLoadImageAttachments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logo.png")
	var encoded bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	data := encoded.Bytes()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}

	images, err := loadImageAttachments([]string{path})
	if err != nil {
		t.Fatalf("loadImageAttachments() error = %v", err)
	}
	if len(images) != 1 || images[0].MIMEType != "image/png" || string(images[0].Data) != string(data) {
		t.Fatalf("images = %+v", images)
	}
}

func TestLoadImageAttachmentsRejectsTruncatedImage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "truncated.png")
	data := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	if _, err := loadImageAttachments([]string{path}); err == nil {
		t.Fatal("loadImageAttachments() accepted a truncated PNG")
	}
}

func TestLoadImageAttachmentsRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.png")
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("create image: %v", err)
	}
	defer file.Close()
	if err := file.Truncate(maxImageBytes + 1); err != nil {
		t.Fatalf("truncate image: %v", err)
	}
	if _, err := loadImageAttachments([]string{path}); err == nil {
		t.Fatal("loadImageAttachments() accepted an oversized image")
	}
}

func TestLoadImageAttachmentsRejectsTooManyFiles(t *testing.T) {
	paths := make([]string, maxImageCount+1)
	for index := range paths {
		paths[index] = "unused.png"
	}
	if _, err := loadImageAttachments(paths); err == nil {
		t.Fatal("loadImageAttachments() accepted too many images")
	}
}

func TestLoadImageAttachmentsReadsWebPDimensions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logo.webp")
	data := make([]byte, 30)
	copy(data[0:4], "RIFF")
	copy(data[8:12], "WEBP")
	copy(data[12:16], "VP8X")
	data[16] = 10
	data[24] = 0
	data[27] = 0
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write webp: %v", err)
	}
	images, err := loadImageAttachments([]string{path})
	if err != nil {
		t.Fatalf("loadImageAttachments() error = %v", err)
	}
	if len(images) != 1 || images[0].MIMEType != "image/webp" {
		t.Fatalf("images = %+v, want WebP attachment", images)
	}
}

func TestLoadImageAttachmentsRejectsUnsupportedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(path, []byte("not an image"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if _, err := loadImageAttachments([]string{path}); err == nil || !strings.Contains(err.Error(), "不支持的图片格式") {
		t.Fatalf("error = %v, want unsupported image error", err)
	}
}
