package files

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"sshworkbench/internal/fault"
	"sshworkbench/internal/ident"
	"sshworkbench/internal/textdoc"
)

const EditorLimit = textdoc.Limit
const UploadLimit = 1024 * 1024 * 1024
const ArchiveLimit = 4 * 1024 * 1024 * 1024
const TreeEntryLimit = 10000
const TreeDepthLimit = 32

type Entry struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Size       int64     `json:"size"`
	Directory  bool      `json:"directory"`
	Symlink    bool      `json:"symlink"`
	Accessible bool      `json:"accessible"`
	Mode       string    `json:"mode"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

type Listing struct {
	Path    string  `json:"path"`
	Entries []Entry `json:"entries"`
}

type Document = textdoc.Document

type Mutation struct {
	Operation string `json:"operation"`
	Path      string `json:"path"`
	Name      string `json:"name,omitempty"`
	Directory bool   `json:"directory,omitempty"`
	Recursive bool   `json:"recursive,omitempty"`
	Mode      string `json:"mode,omitempty"`
	// Expected is the entry count the user confirmed for a recursive delete.
	Expected int `json:"expected,omitempty"`
}

type Usage struct {
	Path        string `json:"path"`
	Files       int    `json:"files"`
	Directories int    `json:"directories"`
	Symlinks    int    `json:"symlinks"`
	Bytes       int64  `json:"bytes"`
	Unsupported int    `json:"unsupported"`
}

type Service struct {
	connector Connector
	targets   TargetSource
	slots     chan struct{}
	uploads   chan struct{}
}

func New(connector Connector, targets TargetSource) *Service {
	return &Service{connector: connector, targets: targets, slots: make(chan struct{}, 8), uploads: make(chan struct{}, 2)}
}

func (service *Service) open(ctx context.Context, owner, terminalID string) (Remote, func(), error) {
	select {
	case service.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, nil, ctx.Err()
	}
	target, err := service.targets.Target(owner, terminalID)
	if err != nil {
		<-service.slots
		return nil, nil, err
	}
	if target.Local {
		<-service.slots
		return nil, nil, fault.New(400, "local_terminal_no_sftp", "本地终端不支持 SFTP 文件面板。")
	}
	dialContext, cancel := context.WithTimeout(ctx, 20*time.Second)
	remote, err := service.connector.OpenFiles(dialContext, target)
	cancel()
	if err != nil {
		<-service.slots
		return nil, nil, err
	}
	stop := context.AfterFunc(ctx, func() { remote.Close() })
	return remote, func() {
		stop()
		remote.Close()
		<-service.slots
	}, nil
}

func (service *Service) List(ctx context.Context, owner, terminalID, directory string) (Listing, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if directory == "" {
		directory = "."
	}
	if err := ValidatePath(directory); err != nil {
		return Listing{}, err
	}
	remote, release, err := service.open(ctx, owner, terminalID)
	if err != nil {
		return Listing{}, err
	}
	defer release()
	canonical, err := remote.RealPath(directory)
	if err != nil {
		return Listing{}, FileError(err)
	}
	if err := ValidatePath(canonical); err != nil {
		return Listing{}, fault.New(415, "unsupported_path_encoding", "远端返回了不支持的目录路径编码。")
	}
	items, err := remote.ReadDir(ctx, canonical)
	if err != nil {
		return Listing{}, FileError(err)
	}
	if len(items) > 10000 {
		return Listing{}, fault.New(413, "directory_too_large", "目录超过 10,000 项，请使用终端筛选。")
	}
	listing := Listing{Path: canonical, Entries: make([]Entry, 0, len(items))}
	for _, item := range items {
		valid := utf8.ValidString(item.Name()) && !strings.ContainsAny(item.Name(), "\x00/\\")
		entry := Entry{Name: item.Name(), Path: path.Join(canonical, item.Name()), Size: item.Size(), Directory: item.IsDir(), Symlink: item.Mode()&fs.ModeSymlink != 0, Accessible: valid, Mode: fmt.Sprintf("%04o", item.Mode().Perm()), ModifiedAt: item.ModTime().UTC()}
		if !valid {
			entry.Name = "[非 UTF-8 或不支持的文件名]"
			entry.Path = ""
		}
		listing.Entries = append(listing.Entries, entry)
	}
	sort.SliceStable(listing.Entries, func(first, second int) bool {
		if listing.Entries[first].Directory != listing.Entries[second].Directory {
			return listing.Entries[first].Directory
		}
		return strings.ToLower(listing.Entries[first].Name) < strings.ToLower(listing.Entries[second].Name)
	})
	return listing, nil
}

func (service *Service) Mutate(ctx context.Context, owner, terminalID string, mutation Mutation) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	if err := ValidatePath(mutation.Path); err != nil {
		return err
	}
	if mutation.Operation == "mkdir" || mutation.Operation == "rename" {
		if err := ValidateName(mutation.Name); err != nil {
			return err
		}
	}
	var mode fs.FileMode
	if mutation.Operation == "chmod" {
		parsed, err := ParseMode(mutation.Mode)
		if err != nil {
			return err
		}
		mode = parsed
	}
	remote, release, err := service.open(ctx, owner, terminalID)
	if err != nil {
		return err
	}
	defer release()
	switch mutation.Operation {
	case "mkdir":
		err = remote.Mkdir(path.Join(mutation.Path, mutation.Name))
	case "rename":
		target := path.Join(path.Dir(mutation.Path), mutation.Name)
		if _, statError := remote.Lstat(target); statError == nil {
			return fault.New(409, "file_exists", "同名目标已经存在，不会覆盖。")
		} else if !errors.Is(statError, fs.ErrNotExist) {
			return FileError(statError)
		}
		err = remote.Rename(mutation.Path, target)
	case "chmod":
		info, statError := remote.Lstat(mutation.Path)
		if statError != nil {
			return FileError(statError)
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fault.New(400, "symlink_unsupported", "不修改符号链接的权限。")
		}
		err = remote.Chmod(mutation.Path, mode)
	case "delete":
		if protected(mutation.Path) {
			return fault.New(400, "protected_path", "不能删除根目录或当前目录。")
		}
		switch {
		case mutation.Directory && mutation.Recursive:
			err = removeTree(ctx, remote, mutation.Path, mutation.Expected)
		case mutation.Directory:
			err = remote.RemoveDirectory(mutation.Path)
		default:
			err = remote.Remove(mutation.Path)
		}
	default:
		return fault.New(400, "invalid_file_operation", "不支持此文件操作。")
	}
	return FileError(err)
}

func realDirectory(remote Remote, directory string) bool {
	info, err := remote.Lstat(directory)
	return err == nil && info.IsDir() && info.Mode()&fs.ModeSymlink == 0
}

func protected(filename string) bool {
	cleaned := path.Clean(filename)
	return cleaned == "/" || cleaned == "." || cleaned == ".."
}

type treeEntry struct {
	path      string
	directory bool
	info      fs.FileInfo
	relative  string
}

// walkTree inventories a directory without following symlinks. It fails
// rather than truncating, so callers never act on a partial picture.
func walkTree(ctx context.Context, remote Remote, root string, usage *Usage) ([]treeEntry, error) {
	info, err := remote.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return nil, fault.New(400, "not_directory", "目标不是目录。")
	}
	var entries []treeEntry
	var visit func(directory, relative string, depth int) error
	visit = func(directory, relative string, depth int) error {
		if depth > TreeDepthLimit {
			return fault.New(413, "tree_too_deep", "目录层级超过 32 层，请使用终端处理。")
		}
		items, err := remote.ReadDir(ctx, directory)
		if err != nil {
			return err
		}
		for _, item := range items {
			if err := ctx.Err(); err != nil {
				return err
			}
			if len(entries) >= TreeEntryLimit {
				return fault.New(413, "tree_too_large", "目录超过 10,000 项，请使用终端处理。")
			}
			name := item.Name()
			if !utf8.ValidString(name) || strings.ContainsAny(name, "\x00/\\") || name == "." || name == ".." {
				usage.Unsupported++
				return fault.New(415, "unsupported_path_encoding", "目录中包含非 UTF-8 或不支持的文件名，已停止，未做任何修改。")
			}
			child := path.Join(directory, name)
			childRelative := path.Join(relative, name)
			switch {
			case item.Mode()&fs.ModeSymlink != 0:
				usage.Symlinks++
				entries = append(entries, treeEntry{path: child, info: item, relative: childRelative})
			case item.IsDir():
				usage.Directories++
				if err := visit(child, childRelative, depth+1); err != nil {
					return err
				}
				entries = append(entries, treeEntry{path: child, directory: true, info: item, relative: childRelative})
			default:
				usage.Files++
				usage.Bytes += item.Size()
				entries = append(entries, treeEntry{path: child, info: item, relative: childRelative})
			}
		}
		return nil
	}
	return entries, visit(root, "", 1)
}

func (service *Service) Inspect(ctx context.Context, owner, terminalID, directory string) (Usage, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := ValidatePath(directory); err != nil {
		return Usage{}, err
	}
	remote, release, err := service.open(ctx, owner, terminalID)
	if err != nil {
		return Usage{}, err
	}
	defer release()
	usage := Usage{Path: directory}
	if _, err := walkTree(ctx, remote, directory, &usage); err != nil {
		return Usage{}, FileError(err)
	}
	return usage, nil
}

// removeTree deletes children before parents and removes symlinks
// themselves, never their targets. The user confirmed a specific entry count;
// if the tree changed since then nothing is deleted.
func removeTree(ctx context.Context, remote Remote, root string, expected int) error {
	var usage Usage
	entries, err := walkTree(ctx, remote, root, &usage)
	if err != nil {
		return err
	}
	if len(entries) != expected {
		return fault.New(409, "tree_changed", "目录内容在确认后发生了变化，未删除任何内容。请重新确认。")
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		// SFTP has no *at calls; re-checking the parent right before each
		// removal narrows the window in which a swapped-in symlink could
		// redirect the delete elsewhere.
		if !realDirectory(remote, path.Dir(entry.path)) {
			return fault.New(409, "tree_changed", "删除过程中目录结构发生变化（出现符号链接或被移除），已停止。部分内容可能已删除，请刷新后检查。")
		}
		if entry.directory {
			err = remote.RemoveDirectory(entry.path)
		} else {
			err = remote.Remove(entry.path)
		}
		if err != nil {
			return fault.New(502, "partial_delete", "删除中途失败，部分内容可能已被删除。请刷新后检查。")
		}
	}
	return remote.RemoveDirectory(root)
}

func (service *Service) ReadDocument(ctx context.Context, owner, terminalID, filename string) (Document, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := ValidatePath(filename); err != nil {
		return Document{}, err
	}
	remote, release, err := service.open(ctx, owner, terminalID)
	if err != nil {
		return Document{}, err
	}
	defer release()
	document, _, err := readDocument(remote, filename)
	return document, err
}

func readDocument(remote Remote, filename string) (Document, fs.FileInfo, error) {
	info, err := remote.Lstat(filename)
	if err != nil {
		return Document{}, nil, FileError(err)
	}
	if !info.Mode().IsRegular() || info.Size() > EditorLimit {
		return Document{}, nil, textdoc.NotEditable()
	}
	reader, err := remote.Open(filename)
	if err != nil {
		return Document{}, nil, FileError(err)
	}
	defer reader.Close()
	data, err := io.ReadAll(io.LimitReader(reader, EditorLimit+1))
	if err != nil {
		return Document{}, nil, FileError(err)
	}
	document, err := textdoc.Decode(filename, data, info.ModTime(), info.Mode())
	return document, info, err
}

func (service *Service) SaveDocument(ctx context.Context, owner, terminalID string, document Document) (Document, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := ValidatePath(document.Path); err != nil {
		return Document{}, err
	}
	if err := textdoc.ValidateContent(document.Content); err != nil {
		return Document{}, err
	}
	remote, release, err := service.open(ctx, owner, terminalID)
	if err != nil {
		return Document{}, err
	}
	defer release()
	current, info, err := readDocument(remote, document.Path)
	if err != nil {
		return Document{}, err
	}
	if document.Version != current.Version {
		return Document{}, textdoc.Conflict()
	}
	content, err := textdoc.Encode(current, document.Content)
	if err != nil {
		return Document{}, err
	}
	// SFTP OPEN cannot set a mode, so a new file starts with the server's
	// default (often 0644) until chmod. Writing inside a fresh 0700 sibling
	// directory keeps other accounts from opening the content meanwhile; the
	// rename then moves it into place on the same filesystem.
	staging := path.Join(path.Dir(document.Path), ".sshwb-"+ident.New())
	if err := remote.Mkdir(staging); err != nil {
		return Document{}, FileError(err)
	}
	defer remote.RemoveDirectory(staging)
	if err := remote.Chmod(staging, 0700); err != nil {
		return Document{}, FileError(err)
	}
	if stagingInfo, err := remote.Lstat(staging); err != nil || !stagingInfo.IsDir() || stagingInfo.Mode()&fs.ModeSymlink != 0 {
		return Document{}, fault.New(409, "staging_unsafe", "无法建立私有的临时目录，未保存。")
	}
	temporary := path.Join(staging, "content")
	writer, err := remote.Create(temporary)
	if err != nil {
		return Document{}, FileError(err)
	}
	defer remote.Remove(temporary)
	_, writeError := writer.Write(content)
	closeError := writer.Close()
	if writeError != nil {
		return Document{}, FileError(writeError)
	}
	if closeError != nil {
		return Document{}, FileError(closeError)
	}
	if err := remote.PreserveAttributes(temporary, info); err != nil {
		return Document{}, FileError(err)
	}
	latest, _, err := readDocument(remote, document.Path)
	if err != nil {
		return Document{}, err
	}
	if latest.Version != current.Version {
		return Document{}, textdoc.Conflict()
	}
	if err := remote.Replace(temporary, document.Path); err != nil {
		return Document{}, fault.New(409, "atomic_replace_unavailable", "服务器不支持安全原子替换或拒绝写入，原文件未主动删除。请下载修改后处理。")
	}
	saved, _, err := readDocument(remote, document.Path)
	return saved, err
}

// Upload writes one file below directory. relative may contain "/" for
// folder uploads; intermediate directories are created but must be real
// directories, never symlinks. Existing files are never overwritten.
func (service *Service) Upload(ctx context.Context, owner, terminalID, directory, relative string, content io.Reader) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	if err := ValidatePath(directory); err != nil {
		return err
	}
	segments, err := ValidateRelative(relative)
	if err != nil {
		return err
	}
	select {
	case service.uploads <- struct{}{}:
		defer func() { <-service.uploads }()
	default:
		return fault.New(429, "upload_limit", "最多同时上传两个文件，请等待已有任务结束。")
	}
	remote, release, err := service.open(ctx, owner, terminalID)
	if err != nil {
		return err
	}
	defer release()
	parent := directory
	for _, segment := range segments[:len(segments)-1] {
		parent = path.Join(parent, segment)
		info, statError := remote.Lstat(parent)
		switch {
		case statError == nil && (!info.IsDir() || info.Mode()&fs.ModeSymlink != 0):
			return fault.New(409, "path_conflict", "上传路径中的 "+segment+" 不是普通目录，已停止。")
		case errors.Is(statError, fs.ErrNotExist):
			if err := remote.Mkdir(parent); err != nil {
				if info, again := remote.Lstat(parent); again != nil || !info.IsDir() {
					return FileError(err)
				}
			}
		case statError != nil:
			return FileError(statError)
		}
	}
	filename := path.Join(parent, segments[len(segments)-1])
	if !realDirectory(remote, parent) {
		return fault.New(409, "path_conflict", "上传目标目录在检查后发生了变化，已停止。")
	}
	if _, statError := remote.Lstat(filename); statError == nil {
		return fault.New(409, "file_exists", "同名文件已经存在，未覆盖。请重命名后上传。")
	} else if !errors.Is(statError, fs.ErrNotExist) {
		return FileError(statError)
	}
	writer, err := remote.Create(filename)
	if err != nil {
		return FileError(err)
	}
	committed := false
	defer func() {
		writer.Close()
		if !committed {
			remote.Remove(filename)
		}
	}()
	count, err := io.Copy(writer, io.LimitReader(content, UploadLimit+1))
	if err != nil {
		return FileError(err)
	}
	if count > UploadLimit {
		return fault.New(413, "upload_too_large", "单个文件不得超过 1 GiB。")
	}
	if err := writer.Close(); err != nil {
		return FileError(err)
	}
	committed = true
	return nil
}

// ValidateRelative accepts a file name or a "/"-separated relative path from
// a folder upload; every segment must be a plain name.
func ValidateRelative(relative string) ([]string, error) {
	if relative == "" || len(relative) > 4096 || strings.HasPrefix(relative, "/") {
		return nil, fault.New(400, "invalid_filename", "文件名不能包含路径分隔符或相对路径。")
	}
	segments := strings.Split(relative, "/")
	if len(segments) > TreeDepthLimit {
		return nil, fault.New(413, "tree_too_deep", "目录层级超过 32 层。")
	}
	for _, segment := range segments {
		if err := ValidateName(segment); err != nil {
			return nil, err
		}
	}
	return segments, nil
}

func (service *Service) Download(ctx context.Context, owner, terminalID, filename string, destination io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	if err := ValidatePath(filename); err != nil {
		return err
	}
	remote, release, err := service.open(ctx, owner, terminalID)
	if err != nil {
		return err
	}
	defer release()
	info, err := remote.Stat(filename)
	if err != nil {
		return FileError(err)
	}
	if !info.Mode().IsRegular() {
		return fault.New(400, "not_regular_file", "此版本只支持下载普通文件。")
	}
	reader, err := remote.Open(filename)
	if err != nil {
		return FileError(err)
	}
	defer reader.Close()
	_, err = io.Copy(destination, reader)
	return FileError(err)
}

// Archive streams a directory as a zip file. Symlinks are skipped (never
// followed); limits are checked on a complete inventory before any bytes are
// written, so an oversized tree fails with a normal error response.
func (service *Service) Archive(ctx context.Context, owner, terminalID, directory string, begin func(string) io.Writer) error {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	if err := ValidatePath(directory); err != nil {
		return err
	}
	if protected(directory) {
		return fault.New(400, "protected_path", "不能打包根目录或当前目录。")
	}
	remote, release, err := service.open(ctx, owner, terminalID)
	if err != nil {
		return err
	}
	defer release()
	var usage Usage
	entries, err := walkTree(ctx, remote, directory, &usage)
	if err != nil {
		return FileError(err)
	}
	if usage.Bytes > ArchiveLimit {
		return fault.New(413, "archive_too_large", "目录总大小超过 4 GiB，请使用终端打包。")
	}
	sort.SliceStable(entries, func(first, second int) bool { return entries[first].relative < entries[second].relative })
	base := path.Base(path.Clean(directory))
	archive := zip.NewWriter(begin(base + ".zip"))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := base + "/" + entry.relative
		switch {
		case entry.info.Mode()&fs.ModeSymlink != 0:
			continue
		case entry.directory:
			if _, err := archive.CreateHeader(&zip.FileHeader{Name: name + "/", Modified: entry.info.ModTime(), Method: zip.Store}); err != nil {
				return err
			}
		default:
			header := &zip.FileHeader{Name: name, Modified: entry.info.ModTime(), Method: zip.Deflate}
			header.SetMode(entry.info.Mode().Perm())
			writer, err := archive.CreateHeader(header)
			if err != nil {
				return err
			}
			reader, err := remote.Open(entry.path)
			if err != nil {
				return err
			}
			_, err = io.Copy(writer, reader)
			reader.Close()
			if err != nil {
				return err
			}
		}
	}
	return archive.Close()
}

func ValidatePath(filename string) error {
	if filename == "" || len(filename) > 4096 || !utf8.ValidString(filename) || strings.ContainsRune(filename, '\x00') {
		return fault.New(400, "invalid_path", "远端路径无效。")
	}
	return nil
}

func ValidateName(name string) error {
	if name == "" || name == "." || name == ".." || len(name) > 255 || !utf8.ValidString(name) || strings.ContainsAny(name, "/\\\x00") {
		return fault.New(400, "invalid_filename", "文件名不能包含路径分隔符或相对路径。")
	}
	return nil
}

func FileError(err error) error {
	if err == nil {
		return nil
	}
	var known *fault.Error
	if errors.As(err, &known) {
		return known
	}
	if errors.Is(err, fs.ErrNotExist) {
		return fault.New(404, "file_not_found", "远端文件或目录不存在。")
	}
	if errors.Is(err, fs.ErrExist) {
		return fault.New(409, "file_exists", "同名文件已经存在，未覆盖。请重命名后上传。")
	}
	if errors.Is(err, fs.ErrPermission) {
		return fault.New(403, "file_permission_denied", "远端账户没有执行此操作的权限。")
	}
	if errors.Is(err, context.Canceled) {
		return fault.New(408, "operation_cancelled", "文件操作已取消。")
	}
	return fault.New(502, "sftp_operation_failed", "SFTP 操作失败，请检查连接、远端权限及目录是否为空。")
}

func ParseMode(value string) (fs.FileMode, error) {
	mode, err := strconv.ParseUint(value, 8, 12)
	if err != nil || mode > 0777 {
		return 0, fault.New(400, "invalid_mode", "权限须为 000–777 的八进制值。")
	}
	return fs.FileMode(mode), nil
}
