package core

import (
	"crypto/rand"
	"encoding/base64"
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
	// fileCacheObjectPrefix is the prefix used to identify file object references.
	fileCacheObjectPrefix = "[FILE OBJECT "

	// fileCacheFailedImageMsg is the message template when caching an image fails.
	// Args: mimeType, error
	fileCacheFailedImageMsg = "Image received (mime: %s) but failed to cache locally: %v"

	// fileCacheFailedResourceMsg is the message template when caching a resource fails.
	// Args: URI, error
	fileCacheFailedResourceMsg = "Resource %s received but failed to cache locally: %v"

	// maxFileRefLength is the maximum length of a base64-encoded file reference.
	// The plain text is ~60-80 chars, base64 adds ~33% overhead.
	maxFileRefLength = 150
)

// encodeFileRef creates a base64-encoded file object reference string.
func encodeFileRef(filename, mimeType string) string {
	plain := fmt.Sprintf("%s%s, mimetype: %s]", fileCacheObjectPrefix, filename, mimeType)
	return base64.StdEncoding.EncodeToString([]byte(plain))
}

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

	name := randomName() + ".tmp"
	path := filepath.Join(fc.tmpDir(), name)

	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}

	fc.mu.Lock()
	fc.trackedFiles = append(fc.trackedFiles, path)
	fc.mu.Unlock()
	fc.logger.Printf("FileCache: saved %d bytes to %s (mime: %s)", len(data), name, mimeType)
	return name, nil
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
	path := filepath.Join(fc.tmpDir(), filename)
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
		Text: encodeFileRef(filename, content.MIMEType),
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
			Text: encodeFileRef(filename, res.MIMEType),
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
			Text: encodeFileRef(filename, mimeType),
		}
	default:
		return nil
	}
}

// ResolveFileArgs walks through tool arguments and replaces any string value
// containing a [FILE OBJECT ...] reference with the cached file content.
func (fc *FileCache) ResolveFileArgs(args map[string]interface{}) {
	for key, val := range args {
		switch v := val.(type) {
		case string:
			if resolved, ok := fc.resolveFileRef(v); ok {
				args[key] = resolved
			}
		case map[string]interface{}:
			fc.ResolveFileArgs(v)
		case []interface{}:
			for i, item := range v {
				if s, ok := item.(string); ok {
					if resolved, ok := fc.resolveFileRef(s); ok {
						v[i] = resolved
					}
				}
			}
		}
	}
}

// resolveFileRef checks if a string is a base64-encoded [FILE OBJECT ...] reference
// and replaces it with the cached file content. The value must be short enough
// to plausibly be an encoded reference. Returns the resolved string and true if
// a replacement was made.
func (fc *FileCache) resolveFileRef(val string) (string, bool) {
	if len(val) > maxFileRefLength {
		return val, false
	}

	// Quick pre-check: base64 strings only contain these characters
	if !isBase64(val) {
		return val, false
	}

	decoded, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		return val, false
	}

	plain := string(decoded)
	if !strings.HasPrefix(plain, fileCacheObjectPrefix) || !strings.HasSuffix(plain, "]") {
		return val, false
	}

	// Extract filename from "[FILE OBJECT filename, mimetype: ...]"
	inner := plain[len(fileCacheObjectPrefix) : len(plain)-1]
	commaIdx := strings.Index(inner, ",")
	if commaIdx < 0 {
		return val, false
	}
	filename := strings.TrimSpace(inner[:commaIdx])

	fc.logger.Printf("resolveFileRef: found FILE OBJECT reference %s in arg (arg length: %d)", filename, len(val))

	content, err := fc.ReadFile(filename)
	if err != nil {
		fc.logger.Printf("Failed to read file ref %s: %v", filename, err)
		return val, false
	}

	fc.logger.Printf("Resolved file ref %s (%d bytes)", filename, len(content))
	return content, true
}

// isBase64 checks if a string looks like valid base64 (only valid characters and proper padding).
func isBase64(s string) bool {
	if len(s) == 0 || len(s)%4 != 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '/' {
			continue
		}
		if c == '=' && i >= len(s)-2 {
			continue
		}
		return false
	}
	return true
}

func randomName() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
