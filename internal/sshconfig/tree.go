package sshconfig

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	ssh_config "github.com/kevinburke/ssh_config"
)

var errNotRegular = errors.New("sshconfig: not a regular file")

type configFile struct {
	source string
	blocks []block
}

type block struct {
	kind    blockKind
	line    int
	host    *ssh_config.Host
	entries []entry
}

// entry is either a directive this package interprets or an Include whose
// files were resolved at load time.
type entry struct {
	key     string
	value   string
	line    int
	include *includeDirective
}

type includeDirective struct {
	line     int
	patterns []string
	files    []*configFile
}

type loader struct {
	home        string
	scope       string
	fileCount   int
	entryCount  int
	fileLimited bool
	diagnostics []Diagnostic
}

// report keeps file-level diagnostics bounded; a config with thousands of
// unmatched Include or Match lines yields one overflow note instead.
func (loader *loader) report(diagnostic Diagnostic) {
	switch {
	case len(loader.diagnostics) < maximumDiagnostics:
		loader.diagnostics = append(loader.diagnostics, diagnostic)
	case len(loader.diagnostics) == maximumDiagnostics:
		loader.diagnostics = append(loader.diagnostics, Diagnostic{
			Level:     LevelInfo,
			Directive: diagnostic.Directive,
			Message:   "诊断信息过多，其余已省略。",
		})
	}
}

func (loader *loader) parse(content []byte, source string) (*configFile, error) {
	prepared, err := prepare(content)
	if err != nil {
		return nil, err
	}
	config, err := decode(prepared.text)
	if err != nil {
		return nil, err
	}
	file, entries, err := assemble(source, prepared, config)
	if err != nil {
		return nil, err
	}
	if loader.entryCount+entries > maximumEntries {
		return nil, fmt.Errorf("%w: more than %d host patterns and directives", ErrTooLarge, maximumEntries)
	}
	loader.entryCount += entries
	for _, diagnostic := range prepared.diagnostics {
		diagnostic.Source = source
		loader.report(diagnostic)
	}
	return file, nil
}

func decode(text []byte) (config *ssh_config.Config, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			config, err = nil, fmt.Errorf("%w: %v", ErrInvalid, recovered)
		}
	}()
	config, err = ssh_config.DecodeBytes(text)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return config, nil
}

// assemble pairs library blocks with the prepared layout and keeps only the
// directives this package interprets. It fails rather than guess when the
// library disagrees with the layout, because a misaligned block or a lost
// Include would silently change which values apply.
func assemble(source string, prepared preparedText, config *ssh_config.Config) (*configFile, int, error) {
	if len(config.Hosts) != len(prepared.blocks)+1 {
		return nil, 0, fmt.Errorf("%w: unexpected block layout", ErrInvalid)
	}
	file := &configFile{source: source}
	entries := 0
	includes := 0
	for index, host := range config.Hosts {
		current := block{kind: blockImplicit, host: host}
		if index > 0 {
			current.kind = prepared.blocks[index-1].kind
			current.line = prepared.blocks[index-1].line
		}
		if current.kind == blockInert && !inertPattern(host) {
			return nil, 0, fmt.Errorf("%w: unexpected block layout", ErrInvalid)
		}
		entries += 1 + len(host.Patterns)
		for _, node := range host.Nodes {
			switch node := node.(type) {
			case *ssh_config.KV:
				key := strings.ToLower(node.Key)
				if _, interpreted := directiveNames[key]; interpreted {
					current.entries = append(current.entries, entry{key: key, value: node.Value, line: node.Pos().Line})
					entries++
				}
			case *ssh_config.Empty:
				patterns, isInclude := prepared.includes[node.Pos().Line]
				if isInclude && node.Comment == includeMarker {
					include := &includeDirective{line: node.Pos().Line, patterns: patterns}
					current.entries = append(current.entries, entry{key: "include", line: include.line, include: include})
					entries += 1 + len(patterns)
					includes++
				}
			}
		}
		file.blocks = append(file.blocks, current)
	}
	if includes != len(prepared.includes) {
		return nil, 0, fmt.Errorf("%w: unexpected include layout", ErrInvalid)
	}
	return file, entries, nil
}

func inertPattern(host *ssh_config.Host) bool {
	return len(host.Patterns) == 1 && strings.HasPrefix(host.Patterns[0].String(), inertMatchPrefix)
}

// followIncludes loads the files named by every Include that can apply.
// Includes inside unsupported Match blocks are skipped because those blocks
// never apply. OpenSSH also reads includes inside non-matching Host blocks
// (it parses them without applying values), so those files are loaded and
// the evaluator decides per alias whether they contribute.
func (loader *loader) followIncludes(file *configFile, depth int, ancestors []string) {
	for _, current := range file.blocks {
		if current.kind == blockInert {
			continue
		}
		for _, item := range current.entries {
			if item.include != nil {
				loader.followInclude(file, item.include, depth, ancestors)
			}
		}
	}
}

func (loader *loader) followInclude(parent *configFile, include *includeDirective, depth int, ancestors []string) {
	diagnose := func(level Level, message string) {
		loader.report(Diagnostic{Level: level, Directive: "Include", Message: message, Source: parent.source, Line: include.line})
	}
	if loader.scope == "" {
		diagnose(LevelWarning, "上传的配置不会读取 Include 文件。")
		return
	}
	if len(include.patterns) == 0 {
		diagnose(LevelWarning, "Include 缺少文件路径。")
		return
	}
	if depth >= maximumIncludeDepth {
		diagnose(LevelWarning, "Include 嵌套超过 5 层，已跳过。")
		return
	}
	for _, pattern := range include.patterns {
		matches, level, message := loader.expandPattern(pattern)
		if message != "" {
			diagnose(level, message)
			continue
		}
		for _, match := range matches {
			child, level, message := loader.loadIncluded(match, depth+1, ancestors)
			if message != "" {
				diagnose(level, message)
				continue
			}
			if child != nil {
				include.files = append(include.files, child)
			}
		}
	}
}

// expandPattern resolves an Include pattern the way OpenSSH does for user
// configuration (~/ is the home directory, relative paths live in ~/.ssh).
func (loader *loader) expandPattern(pattern string) ([]string, Level, string) {
	var path string
	switch {
	case pattern == "~" || strings.HasPrefix(pattern, "~/"):
		path = filepath.Join(loader.home, pattern[1:])
	case strings.HasPrefix(pattern, "~"):
		return nil, LevelWarning, "不支持 ~user 形式的 Include 路径，已跳过。"
	case filepath.IsAbs(pattern):
		path = filepath.Clean(pattern)
	default:
		path = filepath.Join(loader.home, ".ssh", pattern)
	}
	directory := filepath.Dir(path)
	for hasGlobMeta(directory) {
		directory = filepath.Dir(directory)
	}
	if _, err := filepath.EvalSymlinks(directory); err != nil {
		return nil, LevelInfo, "Include 没有匹配到文件。"
	}
	matches, err := filepath.Glob(path)
	if err != nil {
		return nil, LevelWarning, "Include 路径模式无效，已跳过。"
	}
	if len(matches) == 0 {
		return nil, LevelInfo, "Include 没有匹配到文件。"
	}
	sort.Strings(matches)
	return matches, "", ""
}

func hasGlobMeta(path string) bool {
	meta := `*?[`
	if filepath.Separator != '\\' {
		meta = `*?[\`
	}
	return strings.ContainsAny(path, meta)
}

// loadIncluded counts every matched path, including rejected ones, against
// the file limit so that a broad glob stays bounded.
func (loader *loader) loadIncluded(path string, depth int, ancestors []string) (*configFile, Level, string) {
	if loader.fileCount >= maximumFiles {
		if loader.fileLimited {
			return nil, "", ""
		}
		loader.fileLimited = true
		return nil, LevelWarning, "Include 文件超过 32 个，其余已跳过。"
	}
	loader.fileCount++
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, LevelWarning, "Include 文件无法访问，已跳过。"
	}
	if slices.Contains(ancestors, real) {
		return nil, LevelWarning, "Include 形成循环，已跳过。"
	}
	content, err := readConfigFile(real)
	switch {
	case errors.Is(err, errNotRegular):
		return nil, LevelBlocked, "Include 目标不是普通文件，已跳过。"
	case errors.Is(err, ErrTooLarge):
		return nil, LevelBlocked, "Include 文件超过 1 MiB，已跳过。"
	case err != nil:
		return nil, LevelWarning, "Include 文件无法读取，已跳过。"
	}
	child, err := loader.parse(content, loader.displayName(real))
	switch {
	case errors.Is(err, ErrTooLarge):
		return nil, LevelWarning, "Include 文件包含过长的行或过多条目，已跳过。"
	case err != nil:
		return nil, LevelWarning, "Include 文件无法解析，已跳过。"
	}
	loader.followIncludes(child, depth, append(slices.Clip(ancestors), real))
	return child, "", ""
}

func (loader *loader) displayName(real string) string {
	relative, err := filepath.Rel(loader.scope, real)
	if err != nil {
		return filepath.Base(real)
	}
	return filepath.ToSlash(relative)
}

// within compares real paths component-wise so that a sibling such as
// "~/.ssh-other" is not mistaken for a child of "~/.ssh".
func within(directory, path string) bool {
	relative, err := filepath.Rel(directory, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

// readConfigFile checks the type before opening so a FIFO cannot block the
// open, then re-checks the opened file and bounds the read.
func readConfigFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errNotRegular
	}
	if info.Size() > MaxConfigSize {
		return nil, ErrTooLarge
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() {
		return nil, errNotRegular
	}
	content, err := io.ReadAll(io.LimitReader(file, MaxConfigSize+1))
	if err != nil {
		return nil, err
	}
	if len(content) > MaxConfigSize {
		return nil, ErrTooLarge
	}
	return content, nil
}
