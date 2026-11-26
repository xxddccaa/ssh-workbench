package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"sshworkbench/internal/bootstrap"
)

const daemonChildEnv = "SSHW_DAEMON_CHILD"

type daemonMessage struct {
	Ready          *bootstrap.Ready `json:"ready,omitempty"`
	Existing       *bootstrap.Ready `json:"existing,omitempty"`
	AlreadyRunning bool             `json:"alreadyRunning,omitempty"`
	Error          string           `json:"error,omitempty"`
}

func openLogFile(dataDir string) (*os.File, string, error) {
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		return nil, "", err
	}
	logPath := filepath.Join(dataDir, "sshw.log")
	if info, err := os.Stat(logPath); err == nil && info.Size() > 10<<20 {
		_ = os.Rename(logPath, logPath+".1")
	}
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, "", fmt.Errorf("open daemon log %s: %w", logPath, err)
	}
	return file, logPath, nil
}

func filteredEnv(environ []string, stripKeys ...string) []string {
	env := make([]string, 0, len(environ))
	for _, entry := range environ {
		skip := false
		for _, key := range stripKeys {
			if strings.HasPrefix(entry, key+"=") {
				skip = true
				break
			}
		}
		if !skip {
			env = append(env, entry)
		}
	}
	return env
}
