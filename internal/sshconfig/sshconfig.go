// Package sshconfig imports a declared subset of the OpenSSH client
// configuration into host candidates. It never executes commands, never reads
// key material and only follows Include directives inside the user's ~/.ssh
// directory. Everything it cannot represent faithfully is reported as a
// diagnostic instead of being silently dropped.
package sshconfig

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
)

type Level string

const (
	LevelInfo    Level = "info"
	LevelWarning Level = "warning"
	LevelBlocked Level = "blocked"
)

type Diagnostic struct {
	Level     Level  `json:"level"`
	Directive string `json:"directive"`
	Message   string `json:"message"`
	Source    string `json:"source,omitempty"`
	Line      int    `json:"line,omitempty"`
}

type Jump struct {
	Alias    string `json:"alias,omitempty"`
	User     string `json:"user,omitempty"`
	Hostname string `json:"hostname"`
	Port     int    `json:"port"`
}

type Candidate struct {
	Alias          string       `json:"alias"`
	Hostname       string       `json:"hostname"`
	Port           int          `json:"port"`
	User           string       `json:"user"`
	IdentityFiles  []string     `json:"identityFiles,omitempty"`
	IdentitiesOnly bool         `json:"identitiesOnly"`
	UseAgent       bool         `json:"useAgent"`
	ProxyCommand   string       `json:"proxyCommand,omitempty"`
	Jump           *Jump        `json:"jump,omitempty"`
	Importable     bool         `json:"importable"`
	Diagnostics    []Diagnostic `json:"diagnostics"`
}

type Result struct {
	Candidates  []Candidate  `json:"candidates"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// MaxConfigSize bounds every configuration file, whether read from disk or
// uploaded.
const MaxConfigSize = 1 << 20

const (
	maximumIncludeDepth = 5
	maximumFiles        = 32
	maximumCandidates   = 1000
	// The library lexer is quadratic in line length, so long lines are
	// rejected before parsing.
	maximumLineLength = 8 << 10
	// maximumEntries bounds blocks, Host patterns, Include patterns and
	// interpreted directives across all files, which bounds the cost of
	// expanding includes and evaluating every candidate.
	maximumEntries = 20000
	// maximumDiagnostics bounds file-level diagnostics.
	maximumDiagnostics = 200
)

var (
	ErrTooLarge = errors.New("sshconfig: configuration exceeds size limits")
	ErrInvalid  = errors.New("sshconfig: configuration cannot be parsed")
)

// ParseLocal reads <home>/.ssh/config. A missing file yields an empty result.
// Include directives are followed only to regular files whose real path is
// inside the real path of <home>/.ssh.
func ParseLocal(home string) (Result, error) {
	if !filepath.IsAbs(home) {
		return Result{}, errors.New("sshconfig: home directory must be an absolute path")
	}
	directory := filepath.Join(home, ".ssh")
	rootPath := filepath.Join(directory, "config")
	content, err := readConfigFile(rootPath)
	if errors.Is(err, fs.ErrNotExist) {
		return emptyResult(), nil
	}
	if errors.Is(err, errNotRegular) {
		return Result{}, fmt.Errorf("%w: ~/.ssh/config is not a regular file", ErrInvalid)
	}
	if err != nil {
		return Result{}, fmt.Errorf("sshconfig: read config: %w", err)
	}
	scope, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return Result{}, fmt.Errorf("sshconfig: resolve ~/.ssh: %w", err)
	}
	rootReal, err := filepath.EvalSymlinks(rootPath)
	if err != nil {
		return Result{}, fmt.Errorf("sshconfig: resolve config: %w", err)
	}
	loader := &loader{home: home, scope: scope, fileCount: 1}
	root, err := loader.parse(content, "config")
	if err != nil {
		return Result{}, err
	}
	loader.followIncludes(root, 0, []string{rootReal})
	return loader.result(root), nil
}

// ParseText parses an uploaded configuration. Include directives are never
// followed and identity paths are returned without ~ expansion because no
// home directory applies to uploaded text.
func ParseText(content []byte) (Result, error) {
	if len(content) > MaxConfigSize {
		return Result{}, fmt.Errorf("%w: larger than %d bytes", ErrTooLarge, MaxConfigSize)
	}
	loader := &loader{}
	root, err := loader.parse(content, "")
	if err != nil {
		return Result{}, err
	}
	loader.followIncludes(root, 0, nil)
	return loader.result(root), nil
}

func emptyResult() Result {
	return Result{Candidates: []Candidate{}, Diagnostics: []Diagnostic{}}
}
