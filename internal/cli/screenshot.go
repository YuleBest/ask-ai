package cli

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"time"

	"github.com/YuleBest/ask-ai/internal/chat"
	"github.com/YuleBest/ask-ai/internal/i18n"
	"github.com/kbinani/screenshot"
)

type screenshotCapture func() (image.Image, error)

const screenshotTimeout = 15 * time.Second

var captureScreen screenshotCapture = capturePrimaryDisplay

func capturePrimaryDisplay() (image.Image, error) {
	if screenshot.NumActiveDisplays() == 0 {
		return nil, i18n.New("没有检测到可用显示器")
	}
	return screenshot.CaptureDisplay(0)
}

func captureScreenshotWithTimeout(capture screenshotCapture, timeout time.Duration) (image.Image, error) {
	type result struct {
		image image.Image
		err   error
	}
	done := make(chan result, 1)
	go func() {
		imageData, err := capture()
		done <- result{image: imageData, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case result := <-done:
		return result.image, result.err
	case <-timer.C:
		return nil, i18n.New("截取屏幕超时")
	}
}

func loadScreenshotAttachment(status io.Writer, capture screenshotCapture) (chat.ImageAttachment, error) {
	fmt.Fprintln(status, newOutputStyle(status).gray(i18n.T("正在截取当前屏幕...")))

	img, err := captureScreenshotWithTimeout(capture, screenshotTimeout)
	if err != nil {
		return chat.ImageAttachment{}, i18n.Errorf("截取屏幕失败: %w", err)
	}
	if img == nil {
		return chat.ImageAttachment{}, i18n.New("截图为空")
	}

	bounds := img.Bounds()
	if bounds.Dx() <= 0 || bounds.Dy() <= 0 ||
		bounds.Dx() > maxImageDimension || bounds.Dy() > maxImageDimension {
		return chat.ImageAttachment{}, i18n.Errorf(
			"截图尺寸无效或过大: %dx%d",
			bounds.Dx(),
			bounds.Dy(),
		)
	}

	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		return chat.ImageAttachment{}, i18n.Errorf("编码截图失败: %w", err)
	}
	if encoded.Len() > maxImageBytes {
		return chat.ImageAttachment{}, i18n.Errorf("截图超过 %d 字节限制", maxImageBytes)
	}
	return chat.ImageAttachment{
		MIMEType: "image/png",
		Data:     encoded.Bytes(),
	}, nil
}
