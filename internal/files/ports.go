package files

import (
	"context"
	"io"
	"io/fs"

	"sshworkbench/internal/connections"
)

type Remote interface {
	ReadDir(context.Context, string) ([]fs.FileInfo, error)
	RealPath(string) (string, error)
	Stat(string) (fs.FileInfo, error)
	Lstat(string) (fs.FileInfo, error)
	Open(string) (io.ReadCloser, error)
	Create(string) (io.WriteCloser, error)
	Rename(string, string) error
	Replace(string, string) error
	Remove(string) error
	RemoveDirectory(string) error
	Mkdir(string) error
	Chmod(string, fs.FileMode) error
	PreserveAttributes(string, fs.FileInfo) error
	Close() error
}

type Connector interface {
	OpenFiles(context.Context, connections.Target) (Remote, error)
}

type TargetSource interface {
	Target(string, string) (connections.Target, error)
}
