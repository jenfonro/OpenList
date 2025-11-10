package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/OpenListTeam/OpenList/v4/internal/conf"
)

type UploadMetadata struct {
	Size       int64             `json:"size"`
	SliceSize  int64             `json:"slice_size"`
	ContentMD5 string            `json:"content_md5"`
	SliceMD5   string            `json:"slice_md5"`
	BlockList  []string          `json:"block_list"`
	Extras     map[string]string `json:"extras,omitempty"`
}

type uploadMetadataJSON struct {
	Size       int64             `json:"size"`
	SliceSize  int64             `json:"slice_size"`
	ContentMD5 string            `json:"content_md5"`
	SliceMD5   string            `json:"slice_md5"`
	BlockList  []string          `json:"block_list"`
	Extras     map[string]string `json:"extras,omitempty"`
	UploadURL  string            `json:"upload_url,omitempty"`
	FileSHA1   string            `json:"file_sha1,omitempty"`
}

func (m UploadMetadata) MarshalJSON() ([]byte, error) {
	aux := uploadMetadataJSON{
		Size:       m.Size,
		SliceSize:  m.SliceSize,
		ContentMD5: m.ContentMD5,
		SliceMD5:   m.SliceMD5,
		BlockList:  m.BlockList,
		Extras:     m.Extras,
	}
	return json.Marshal(aux)
}

func (m *UploadMetadata) UnmarshalJSON(data []byte) error {
	var aux uploadMetadataJSON
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	m.Size = aux.Size
	m.SliceSize = aux.SliceSize
	m.ContentMD5 = aux.ContentMD5
	m.SliceMD5 = aux.SliceMD5
	m.BlockList = aux.BlockList
	m.Extras = aux.Extras
	if aux.UploadURL != "" {
		m.SetExtra("upload_url", aux.UploadURL)
	}
	if aux.FileSHA1 != "" {
		m.SetExtra("file_sha1", aux.FileSHA1)
	}
	return nil
}

type UploadCache struct {
	cachedPath   string
	tempFile     string
	keep         map[string]struct{}
	metadata     *UploadMetadata
	metadataPath string
	mu           sync.RWMutex
	retainMeta   bool
}

// NewUploadCache creates a cache holder with an optional existing cached file path.
func NewUploadCache(path string, opts ...UploadCacheOption) *UploadCache {
	uc := &UploadCache{}
	if path != "" {
		uc.cachedPath = normalizePath(path)
	}
	for _, opt := range opts {
		if opt != nil {
			opt(uc)
		}
	}
	return uc
}

// UploadCacheOption configures UploadCache creation.
type UploadCacheOption func(*UploadCache)

// WithMetadataKey configures UploadCache to persist metadata using the provided key.
func WithMetadataKey(key string) UploadCacheOption {
	return func(u *UploadCache) {
		if key == "" {
			return
		}
		u.metadataPath = metadataPathForKey(key)
	}
}

func (u *UploadCache) CachedPath() string {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.cachedPath
}

func (u *UploadCache) SetCachedPath(path string) {
	if path == "" {
		return
	}
	u.mu.Lock()
	u.cachedPath = normalizePath(path)
	u.metadata = nil
	u.mu.Unlock()
}

func (u *UploadCache) RegisterTemp(path string) {
	if path == "" {
		return
	}
	u.mu.Lock()
	u.tempFile = normalizePath(path)
	u.metadata = nil
	u.mu.Unlock()
}

func (u *UploadCache) TempFile() string {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.tempFile
}

func (u *UploadCache) CurrentPath() string {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.currentPathLocked()
}

func (u *UploadCache) currentPathLocked() string {
	if u.tempFile != "" {
		return u.tempFile
	}
	return u.cachedPath
}

func (u *UploadCache) MarkKeep(path string) {
	if path == "" {
		return
	}
	u.mu.Lock()
	if u.keep == nil {
		u.keep = make(map[string]struct{})
	}
	u.keep[normalizePath(path)] = struct{}{}
	u.mu.Unlock()
}

func (u *UploadCache) ShouldKeep(path string) bool {
	if path == "" {
		return false
	}
	nPath := normalizePath(path)
	u.mu.RLock()
	defer u.mu.RUnlock()
	if u.cachedPath != "" && u.cachedPath == nPath {
		return true
	}
	if _, ok := u.keep[nPath]; ok {
		return true
	}
	return false
}

func normalizePath(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func (u *UploadCache) metadataPathLocked() string {
	if u.metadataPath != "" {
		return u.metadataPath
	}
	path := u.currentPathLocked()
	if path == "" {
		return ""
	}
	return path + ".meta"
}

// MetadataPathFor returns the metadata sidecar path for the provided temp file path.
func MetadataPathFor(path string) string {
	if path == "" {
		return ""
	}
	return normalizePath(path) + ".meta"
}

// MetadataPath returns the sidecar metadata path for the cached file, if known.
func (u *UploadCache) MetadataPath() string {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.metadataPathLocked()
}

// Metadata returns a copy of the cached metadata in memory, if any.
func (u *UploadCache) Metadata() *UploadMetadata {
	u.mu.RLock()
	defer u.mu.RUnlock()
	if u.metadata == nil {
		return nil
	}
	return cloneMetadata(u.metadata)
}

func cloneMetadata(meta *UploadMetadata) *UploadMetadata {
	if meta == nil {
		return nil
	}
	bl := make([]string, len(meta.BlockList))
	copy(bl, meta.BlockList)
	var extras map[string]string
	if len(meta.Extras) > 0 {
		extras = make(map[string]string, len(meta.Extras))
		for k, v := range meta.Extras {
			extras[k] = v
		}
	}
	return &UploadMetadata{
		Size:       meta.Size,
		SliceSize:  meta.SliceSize,
		ContentMD5: meta.ContentMD5,
		SliceMD5:   meta.SliceMD5,
		BlockList:  bl,
		Extras:     extras,
	}
}

func (m *UploadMetadata) SetExtra(key, value string) {
	if key == "" {
		return
	}
	if value == "" {
		if m.Extras != nil {
			delete(m.Extras, key)
			if len(m.Extras) == 0 {
				m.Extras = nil
			}
		}
		return
	}
	if m.Extras == nil {
		m.Extras = make(map[string]string)
	}
	m.Extras[key] = value
}

func (m *UploadMetadata) GetExtra(key string) string {
	if m == nil || key == "" {
		return ""
	}
	return m.Extras[key]
}

// LoadMetadata loads metadata from disk into memory if not yet present.
func (u *UploadCache) LoadMetadata() (*UploadMetadata, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.metadata != nil {
		return cloneMetadata(u.metadata), nil
	}
	path := u.metadataPathLocked()
	if path == "" {
		return nil, os.ErrNotExist
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var meta UploadMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	u.metadata = &meta
	return cloneMetadata(u.metadata), nil
}

// SaveMetadata stores metadata in memory and persists it alongside the temp file.
func (u *UploadCache) SaveMetadata(meta *UploadMetadata) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if meta == nil {
		u.metadata = nil
		path := u.metadataPathLocked()
		if path != "" {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	}
	u.metadata = cloneMetadata(meta)
	path := u.metadataPathLocked()
	if path == "" {
		return nil
	}
	data, err := json.Marshal(u.metadata)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// RemoveMetadataFile deletes the persisted metadata sidecar file if present.
func (u *UploadCache) RemoveMetadataFile() error {
	path := u.MetadataPath()
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// RemoveMetadataByPath deletes the metadata sidecar associated with the given temp file path.
func RemoveMetadataByPath(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(MetadataPathFor(path))
}

// MarkRetainMetadata marks the metadata to persist even after task failures.
func (u *UploadCache) MarkRetainMetadata() {
	u.mu.Lock()
	u.retainMeta = true
	u.mu.Unlock()
}

// ShouldRetainMetadata reports whether metadata should persist after failures.
func (u *UploadCache) ShouldRetainMetadata() bool {
	u.mu.RLock()
	defer u.mu.RUnlock()
	return u.retainMeta
}

// RemoveMetadataFileAt deletes the metadata file at the provided path.
func RemoveMetadataFileAt(path string) {
	if path == "" {
		return
	}
	_ = os.Remove(path)
}

// MetadataPathForKey returns the metadata file path derived from the provided key.
func MetadataPathForKey(key string) string {
	return metadataPathForKey(key)
}

func metadataPathForKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		key = fmt.Sprintf("anon-%d", time.Now().UnixNano())
	}
	var b strings.Builder
	for _, r := range key {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') {
			b.WriteRune(r)
			continue
		}
		switch r {
		case '-', '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	safe := b.String()
	if safe == "" {
		safe = fmt.Sprintf("anon-%d", time.Now().UnixNano())
	}
	return filepath.Join(conf.Conf.TempDir, fmt.Sprintf("upload-%s.meta", safe))
}

// UploadCacheFromContext extracts the UploadCache pointer from the provided context, if any.
func UploadCacheFromContext(ctx context.Context) *UploadCache {
	if ctx == nil {
		return nil
	}
	if v, ok := ctx.Value(conf.UploadCacheKey).(*UploadCache); ok {
		return v
	}
	return nil
}
