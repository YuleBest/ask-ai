package cli

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/YuleBest/ask-ai/internal/chat"
	"github.com/YuleBest/ask-ai/internal/i18n"
)

const (
	maxImageBytes     = 20 << 20
	maxImageCount     = 8
	maxImageDimension = 16384
)

func loadImageAttachments(paths []string) ([]chat.ImageAttachment, error) {
	if len(paths) > maxImageCount {
		return nil, i18n.Errorf("图片数量不能超过 %d 张", maxImageCount)
	}
	images := make([]chat.ImageAttachment, 0, len(paths))
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, i18n.Errorf("读取图片 %s 失败: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			return nil, i18n.Errorf("图片不是普通文件: %s", path)
		}
		if info.Size() > maxImageBytes {
			return nil, i18n.Errorf("图片超过 %d 字节限制: %s", maxImageBytes, path)
		}

		file, err := os.Open(path)
		if err != nil {
			return nil, i18n.Errorf("读取图片 %s 失败: %w", path, err)
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxImageBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return nil, i18n.Errorf("读取图片 %s 失败: %w", path, readErr)
		}
		if closeErr != nil {
			return nil, i18n.Errorf("关闭图片 %s 失败: %w", path, closeErr)
		}
		if len(data) > maxImageBytes {
			return nil, i18n.Errorf("图片超过 %d 字节限制: %s", maxImageBytes, path)
		}
		detected := http.DetectContentType(data)
		if !strings.HasPrefix(detected, "image/") {
			return nil, i18n.Errorf("不支持的图片格式: %s (%s)", path, detected)
		}

		var config image.Config
		var format string
		if detected == "image/webp" {
			config, err = webpConfig(data)
			format = "webp"
		} else {
			config, format, err = image.DecodeConfig(bytes.NewReader(data))
		}
		if err != nil {
			return nil, i18n.Errorf("图片格式无效: %s (%w)", path, err)
		}
		mimeType, ok := imageMIME[format]
		if !ok {
			return nil, i18n.Errorf("不支持的图片格式: %s (%s)", path, format)
		}
		if config.Width <= 0 || config.Height <= 0 ||
			config.Width > maxImageDimension || config.Height > maxImageDimension {
			return nil, i18n.Errorf("图片尺寸无效或过大: %s (%dx%d)", path, config.Width, config.Height)
		}
		images = append(images, chat.ImageAttachment{
			MIMEType: mimeType,
			Data:     data,
		})
	}
	return images, nil
}

var imageMIME = map[string]string{
	"gif":  "image/gif",
	"jpeg": "image/jpeg",
	"png":  "image/png",
	"webp": "image/webp",
}

func webpConfig(data []byte) (image.Config, error) {
	if len(data) < 20 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WEBP" {
		return image.Config{}, i18n.New("无效的 WebP 容器")
	}
	chunkSize := int(uint32(data[16]) | uint32(data[17])<<8 | uint32(data[18])<<16 | uint32(data[19])<<24)
	if chunkSize < 5 || 20+chunkSize > len(data) {
		return image.Config{}, i18n.New("无效的 WebP 块")
	}

	switch string(data[12:16]) {
	case "VP8X":
		if chunkSize < 10 || len(data) < 30 {
			return image.Config{}, i18n.New("无效的 WebP VP8X 头")
		}
		return image.Config{
			Width:  1 + readUint24LE(data[24:27]),
			Height: 1 + readUint24LE(data[27:30]),
		}, nil
	case "VP8 ":
		if chunkSize < 10 || len(data) < 30 || data[23] != 0x9d || data[24] != 0x01 || data[25] != 0x2a {
			return image.Config{}, i18n.New("无效的 WebP VP8 头")
		}
		return image.Config{
			Width:  int(uint16(data[26])|uint16(data[27])<<8) & 0x3fff,
			Height: int(uint16(data[28])|uint16(data[29])<<8) & 0x3fff,
		}, nil
	case "VP8L":
		if chunkSize < 5 || len(data) < 25 || data[20] != 0x2f {
			return image.Config{}, i18n.New("无效的 WebP VP8L 头")
		}
		bits := uint32(data[21]) | uint32(data[22])<<8 | uint32(data[23])<<16 | uint32(data[24])<<24
		return image.Config{
			Width:  int(bits&0x3fff) + 1,
			Height: int((bits>>14)&0x3fff) + 1,
		}, nil
	default:
		return image.Config{}, i18n.New("不支持的 WebP 编码")
	}
}

func readUint24LE(data []byte) int {
	return int(uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16)
}
