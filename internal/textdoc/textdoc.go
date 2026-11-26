// Package textdoc holds the editing rules shared by remote and local text
// documents: UTF-8 only, a 2 MiB limit, preserved BOM and line endings, and a
// version derived from content and metadata for conflict detection.
package textdoc

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"strings"
	"time"
	"unicode/utf8"

	"sshworkbench/internal/fault"
)

const Limit = 2 * 1024 * 1024

type Document struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	Version    string `json:"version"`
	LineEnding string `json:"lineEnding"`
	BOM        bool   `json:"bom"`
}

func NotEditable() error {
	return fault.New(413, "not_editable", "仅支持编辑不超过 2 MiB 的 UTF-8 普通文本文件。")
}

// Decode validates raw file bytes and describes their format.
func Decode(filename string, data []byte, modified time.Time, mode fs.FileMode) (Document, error) {
	if len(data) > Limit || !utf8.Valid(data) || bytes.ContainsRune(data, '\x00') {
		return Document{}, fault.New(415, "unsupported_encoding", "文件不是受支持的 UTF-8 文本，请下载后处理。")
	}
	versionInput := append(append([]byte(nil), data...), []byte(fmt.Sprintf("\x00%d:%d", modified.UnixNano(), mode))...)
	digest := sha256.Sum256(versionInput)
	document := Document{Path: filename, Version: hex.EncodeToString(digest[:]), LineEnding: "lf", BOM: bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf})}
	document.Content = strings.TrimPrefix(string(data), "\ufeff")
	withoutCRLF := strings.ReplaceAll(document.Content, "\r\n", "")
	if strings.Contains(document.Content, "\r\n") {
		document.LineEnding = "crlf"
		if strings.Contains(withoutCRLF, "\n") {
			document.LineEnding = "mixed"
		}
	}
	if strings.Contains(withoutCRLF, "\r") {
		document.LineEnding = "mixed"
	}
	return document, nil
}

func ValidateContent(content string) error {
	if len(content) > Limit || !utf8.ValidString(content) || strings.ContainsRune(content, '\x00') {
		return fault.New(413, "document_too_large", "文本无效或超过 2 MiB 编辑限制。")
	}
	return nil
}

// Encode applies the current file's BOM and line ending to edited content.
// Mixed line endings are refused rather than silently normalized.
func Encode(current Document, content string) ([]byte, error) {
	if current.LineEnding == "mixed" {
		return nil, fault.New(409, "mixed_line_endings", "文件混用了换行符，当前版本只读，避免静默改写格式。")
	}
	if err := ValidateContent(content); err != nil {
		return nil, err
	}
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if current.LineEnding == "crlf" {
		content = strings.ReplaceAll(content, "\n", "\r\n")
	}
	if current.BOM {
		content = "\ufeff" + content
	}
	if len(content) > Limit {
		return nil, fault.New(413, "document_too_large", "恢复原始换行符后超过 2 MiB，未保存。")
	}
	return []byte(content), nil
}

func Conflict() error {
	return fault.New(409, "document_conflict", "文件已在别处被修改，未覆盖。请重新加载或复制修改后另行合并。")
}
