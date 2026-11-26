// Package localdocs edits local files that the current OS user explicitly
// registered through the local control channel (`sshworkbench edit`). The web
// API can only refer to registered opaque identifiers, never to paths.
package localdocs

import (
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"sshworkbench/internal/fault"
	"sshworkbench/internal/ident"
	"sshworkbench/internal/textdoc"
)

const (
	registrationLimit    = 64
	registrationLifetime = 12 * time.Hour
)

type Registration struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type Service struct {
	mu        sync.Mutex
	saving    sync.Mutex
	documents map[string]Registration
}

func New() *Service {
	return &Service{documents: make(map[string]Registration)}
}

// Register resolves symlinks once, so later reads and writes target the real
// file the user named and a swapped link cannot redirect them.
func (service *Service) Register(filename string) (Registration, error) {
	if !filepath.IsAbs(filename) {
		return Registration{}, fault.New(400, "absolute_path_required", "请使用绝对路径。")
	}
	resolved, err := filepath.EvalSymlinks(filename)
	if err != nil {
		return Registration{}, fault.New(404, "local_file_not_found", "本机文件不存在或无法访问。")
	}
	if _, err := inspect(resolved); err != nil {
		return Registration{}, err
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	now := time.Now()
	for identifier, registration := range service.documents {
		if now.After(registration.ExpiresAt) {
			delete(service.documents, identifier)
		}
	}
	if len(service.documents) >= registrationLimit {
		return Registration{}, fault.New(429, "too_many_documents", "已打开的本机文件过多，请稍后再试。")
	}
	registration := Registration{ID: ident.New(), Path: resolved, ExpiresAt: now.Add(registrationLifetime)}
	service.documents[registration.ID] = registration
	return registration, nil
}

func (service *Service) lookup(identifier string) (Registration, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	registration, exists := service.documents[identifier]
	if !exists || time.Now().After(registration.ExpiresAt) {
		delete(service.documents, identifier)
		return Registration{}, fault.New(404, "local_document_not_found", "本机文件授权不存在或已过期，请重新运行 sshworkbench edit。")
	}
	return registration, nil
}

func inspect(filename string) (os.FileInfo, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return nil, fault.New(404, "local_file_not_found", "本机文件不存在或无法访问。")
	}
	if !info.Mode().IsRegular() || info.Size() > textdoc.Limit {
		return nil, textdoc.NotEditable()
	}
	return info, nil
}

func read(filename string) (textdoc.Document, os.FileInfo, error) {
	info, err := inspect(filename)
	if err != nil {
		return textdoc.Document{}, nil, err
	}
	file, err := os.Open(filename)
	if err != nil {
		return textdoc.Document{}, nil, fault.New(403, "local_file_unreadable", "当前用户无法读取此文件。")
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, textdoc.Limit+1))
	if err != nil {
		return textdoc.Document{}, nil, fault.New(403, "local_file_unreadable", "当前用户无法读取此文件。")
	}
	document, err := textdoc.Decode(filename, data, info.ModTime(), info.Mode())
	return document, info, err
}

func (service *Service) Read(identifier string) (textdoc.Document, error) {
	registration, err := service.lookup(identifier)
	if err != nil {
		return textdoc.Document{}, err
	}
	document, _, err := read(registration.Path)
	return document, err
}

// Save writes a sibling temporary file, flushes it, re-checks the version
// and atomically renames it over the original. Permission bits are kept;
// ownership, ACLs, extended attributes and hard links are not preserved.
func (service *Service) Save(identifier string, edited textdoc.Document) (textdoc.Document, error) {
	// Serialize saves so two windows cannot both pass the version re-check
	// and race their renames.
	service.saving.Lock()
	defer service.saving.Unlock()
	registration, err := service.lookup(identifier)
	if err != nil {
		return textdoc.Document{}, err
	}
	current, info, err := read(registration.Path)
	if err != nil {
		return textdoc.Document{}, err
	}
	if edited.Version != current.Version {
		return textdoc.Document{}, textdoc.Conflict()
	}
	content, err := textdoc.Encode(current, edited.Content)
	if err != nil {
		return textdoc.Document{}, err
	}
	temporary, err := os.CreateTemp(filepath.Dir(registration.Path), ".sshwb-*.tmp")
	if err != nil {
		return textdoc.Document{}, fault.New(403, "local_write_denied", "无法在文件所在目录创建临时文件，未修改原文件。")
	}
	committed := false
	defer func() {
		if !committed {
			temporary.Close()
			os.Remove(temporary.Name())
		}
	}()
	if _, err := temporary.Write(content); err != nil {
		return textdoc.Document{}, fault.New(507, "local_write_failed", "写入临时文件失败，未修改原文件。")
	}
	if err := temporary.Sync(); err != nil {
		return textdoc.Document{}, fault.New(507, "local_write_failed", "写入临时文件失败，未修改原文件。")
	}
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		return textdoc.Document{}, fault.New(403, "local_write_denied", "无法保留原文件权限，未修改原文件。")
	}
	if err := temporary.Close(); err != nil {
		return textdoc.Document{}, fault.New(507, "local_write_failed", "写入临时文件失败，未修改原文件。")
	}
	latest, _, err := read(registration.Path)
	if err != nil {
		return textdoc.Document{}, err
	}
	if latest.Version != current.Version {
		return textdoc.Document{}, textdoc.Conflict()
	}
	if err := os.Rename(temporary.Name(), registration.Path); err != nil {
		return textdoc.Document{}, fault.New(409, "local_replace_failed", "无法替换原文件，原文件未修改。")
	}
	committed = true
	syncDirectory(filepath.Dir(registration.Path))
	saved, _, err := read(registration.Path)
	return saved, err
}
