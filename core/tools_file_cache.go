package core

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gelembjuk/cleverchatty/core/history"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	// fileCacheSavedImageMsg is the message template for a cached image.
	// Args: cache path, mimeType, cache path
	fileCacheSavedImageMsg = "Binary data was cached to local path %s (MimeType: %s). " +
		"The actual data is large and NOT included in this message. " +
		"IMPORTANT: Do NOT try to generate or guess the data content. " +
		"To pass this data to another tool, you MUST use exactly file:%s as the argument value. " +
		"The system will automatically replace this reference with the real data before calling the tool."

	// fileCacheSavedResourceMsg is the message template for a cached resource.
	// Args: cache path, URI, mimeType, cache path
	fileCacheSavedResourceMsg = "Binary data was cached to local path %s (Original URI: %s, MimeType: %s). " +
		"The actual data is large and NOT included in this message. " +
		"IMPORTANT: Do NOT try to generate or guess the data content. " +
		"To pass this data to another tool, you MUST use exactly file:%s as the argument value. " +
		"The system will automatically replace this reference with the real data before calling the tool."

	// fileCacheFailedImageMsg is the message template when caching an image fails.
	// Args: mimeType, error
	fileCacheFailedImageMsg = "Image received (mime: %s) but failed to cache locally: %v"

	// fileCacheFailedResourceMsg is the message template when caching a resource fails.
	// Args: URI, error
	fileCacheFailedResourceMsg = "Resource %s received but failed to cache locally: %v"
)

type FileCache struct {
	workDir      string
	logger       *log.Logger
	trackedFiles []string
	mu           sync.Mutex
}

func NewFileCache(workDir string, logger *log.Logger) *FileCache {
	if workDir == "" {
		workDir = "."
	}
	return &FileCache{
		workDir: workDir,
		logger:  logger,
	}
}

func (fc *FileCache) tmpDir() string {
	return filepath.Join(fc.workDir, "tmp")
}

func (fc *FileCache) ensureTmpDir() error {
	return os.MkdirAll(fc.tmpDir(), 0755)
}

// SaveContent saves raw bytes to a temp file and returns the filename (relative to workDir).
func (fc *FileCache) SaveContent(data []byte, mimeType string) (string, error) {
	if err := fc.ensureTmpDir(); err != nil {
		return "", fmt.Errorf("failed to create tmp dir: %w", err)
	}

	ext := extensionForMIME(mimeType)
	name := randomName() + ext
	path := filepath.Join(fc.tmpDir(), name)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}

	relPath := filepath.Join("tmp", name)
	fc.mu.Lock()
	fc.trackedFiles = append(fc.trackedFiles, path)
	fc.mu.Unlock()
	fc.logger.Printf("FileCache: saved %d bytes to %s (mime: %s)", len(data), relPath, mimeType)
	return relPath, nil
}

// Cleanup removes all temp files created during this session.
func (fc *FileCache) Cleanup() {
	fc.mu.Lock()
	files := fc.trackedFiles
	fc.trackedFiles = nil
	fc.mu.Unlock()

	for _, path := range files {
		if err := os.Remove(path); err != nil {
			fc.logger.Printf("FileCache: failed to remove %s: %v", path, err)
		} else {
			fc.logger.Printf("FileCache: removed %s", path)
		}
	}
}

// SaveBase64Content saves base64-encoded data directly to a temp file without decoding.
// The data is stored as-is since tools typically exchange base64.
func (fc *FileCache) SaveBase64Content(b64Data string, mimeType string) (string, error) {
	return fc.SaveContent([]byte(b64Data), mimeType)
}

// ReadFile reads a file from the cache directory and returns its content as a string.
func (fc *FileCache) ReadFile(filename string) (string, error) {
	path := filepath.Join(fc.workDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %w", filename, err)
	}
	return string(data), nil
}

// HandleImageContent saves an MCP ImageContent to a temp file and returns a text content describing the stored file.
func (fc *FileCache) HandleImageContent(content mcp.ImageContent) history.Content {
	filename, err := fc.SaveBase64Content(content.Data, content.MIMEType)
	if err != nil {
		fc.logger.Printf("Failed to save image content to file: %v", err)
		return history.TextContent{
			Type: "text",
			Text: fmt.Sprintf(fileCacheFailedImageMsg, content.MIMEType, err),
		}
	}
	return history.TextContent{
		Type: "text",
		Text: fmt.Sprintf(fileCacheSavedImageMsg, filename, content.MIMEType, filename),
	}
}

// HandleEmbeddedResource saves an MCP EmbeddedResource to a temp file and returns a text content describing the stored file.
func (fc *FileCache) HandleEmbeddedResource(content mcp.EmbeddedResource) history.Content {
	switch res := content.Resource.(type) {
	case mcp.BlobResourceContents:
		filename, err := fc.SaveBase64Content(res.Blob, res.MIMEType)
		if err != nil {
			fc.logger.Printf("Failed to save blob resource to file: %v", err)
			return history.TextContent{
				Type: "text",
				Text: fmt.Sprintf(fileCacheFailedResourceMsg, res.URI, err),
			}
		}
		return history.TextContent{
			Type: "text",
			Text: fmt.Sprintf(fileCacheSavedResourceMsg, filename, res.URI, res.MIMEType, filename),
		}
	case mcp.TextResourceContents:
		mimeType := res.MIMEType
		if mimeType == "" {
			mimeType = "text/plain"
		}
		filename, err := fc.SaveContent([]byte(res.Text), mimeType)
		if err != nil {
			fc.logger.Printf("Failed to save text resource to file: %v", err)
			return history.TextContent{
				Type: "text",
				Text: fmt.Sprintf(fileCacheFailedResourceMsg, res.URI, err),
			}
		}
		return history.TextContent{
			Type: "text",
			Text: fmt.Sprintf(fileCacheSavedResourceMsg, filename, res.URI, mimeType, filename),
		}
	default:
		return nil
	}
}

func randomName() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func extensionForMIME(mimeType string) string {
	switch {
	case strings.HasPrefix(mimeType, "image/png"):
		return ".png"
	case strings.HasPrefix(mimeType, "image/jpeg"):
		return ".jpg"
	case strings.HasPrefix(mimeType, "image/gif"):
		return ".gif"
	case strings.HasPrefix(mimeType, "image/webp"):
		return ".webp"
	case strings.HasPrefix(mimeType, "image/svg"):
		return ".svg"
	case strings.HasPrefix(mimeType, "application/pdf"):
		return ".pdf"
	case strings.HasPrefix(mimeType, "text/plain"):
		return ".txt"
	case strings.HasPrefix(mimeType, "text/html"):
		return ".html"
	case strings.HasPrefix(mimeType, "application/json"):
		return ".json"
	default:
		return ".bin"
	}
}
