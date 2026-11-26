package sshtransport

import (
	"context"
	"io"
	"io/fs"
	"os"
	"sync"

	"github.com/pkg/sftp"

	"sshworkbench/internal/connections"
	"sshworkbench/internal/fault"
	"sshworkbench/internal/files"
)

// OpenFiles gives each file operation its own transport so transfers cannot
// starve terminal output. Hosts that need a person to authenticate
// (keyboard-interactive) cannot be re-dialed in the background; for those the
// SFTP subsystem runs as an extra channel on the terminal's own connection.
func (connector *Connector) OpenFiles(ctx context.Context, target connections.Target) (files.Remote, error) {
	if target.RequiresUser() {
		shared, ok := target.Shared.(*lease)
		if !ok || !shared.Alive() {
			return nil, fault.New(409, "terminal_transport_closed", "此主机使用交互式认证，文件操作依赖终端连接；终端连接已断开，请重新连接。")
		}
		return openSharedFiles(ctx, shared)
	}
	transport, err := connector.dial(ctx, target)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { transport.Close() })
	defer stop()
	filesystem, err := sftp.NewClient(transport.client)
	if err != nil {
		transport.Close()
		return nil, fault.New(502, "sftp_unavailable", "此主机没有可用的 SFTP 子系统，终端仍可独立使用。")
	}
	return &remoteFiles{client: filesystem, release: func() { transport.Close() }}, nil
}

// openSharedFiles runs the SFTP subsystem as an extra channel on an
// existing connection. The channel is opened explicitly so a server that
// never answers SFTP initialization is abandoned when ctx ends.
func openSharedFiles(ctx context.Context, shared *lease) (files.Remote, error) {
	unavailable := fault.New(502, "sftp_unavailable", "此主机没有可用的 SFTP 子系统，终端仍可独立使用。")
	session, err := shared.current().NewSession()
	if err != nil {
		return nil, unavailable
	}
	stop := context.AfterFunc(ctx, func() { session.Close() })
	defer stop()
	writer, writerError := session.StdinPipe()
	reader, readerError := session.StdoutPipe()
	if writerError != nil || readerError != nil || session.RequestSubsystem("sftp") != nil {
		session.Close()
		return nil, unavailable
	}
	filesystem, err := sftp.NewClientPipe(reader, writer)
	if err != nil || ctx.Err() != nil {
		session.Close()
		return nil, unavailable
	}
	return &remoteFiles{client: filesystem, release: func() { session.Close() }}, nil
}

type remoteFiles struct {
	client  *sftp.Client
	release func()
	once    sync.Once
}

func (remote *remoteFiles) ReadDir(ctx context.Context, directory string) ([]fs.FileInfo, error) {
	return remote.client.ReadDirContext(ctx, directory)
}
func (remote *remoteFiles) RealPath(directory string) (string, error) {
	return remote.client.RealPath(directory)
}
func (remote *remoteFiles) Stat(filename string) (fs.FileInfo, error) {
	return remote.client.Stat(filename)
}
func (remote *remoteFiles) Lstat(filename string) (fs.FileInfo, error) {
	return remote.client.Lstat(filename)
}
func (remote *remoteFiles) Open(filename string) (io.ReadCloser, error) {
	return remote.client.Open(filename)
}
func (remote *remoteFiles) Create(filename string) (io.WriteCloser, error) {
	return remote.client.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
}
func (remote *remoteFiles) Rename(source, destination string) error {
	return remote.client.Rename(source, destination)
}
func (remote *remoteFiles) Replace(source, destination string) error {
	return remote.client.PosixRename(source, destination)
}
func (remote *remoteFiles) Remove(filename string) error { return remote.client.Remove(filename) }
func (remote *remoteFiles) RemoveDirectory(directory string) error {
	return remote.client.RemoveDirectory(directory)
}
func (remote *remoteFiles) Mkdir(directory string) error { return remote.client.Mkdir(directory) }
func (remote *remoteFiles) Chmod(filename string, mode fs.FileMode) error {
	return remote.client.Chmod(filename, mode)
}
func (remote *remoteFiles) PreserveAttributes(filename string, original fs.FileInfo) error {
	if err := remote.client.Chmod(filename, original.Mode().Perm()); err != nil {
		return err
	}
	current, err := remote.client.Stat(filename)
	if err != nil {
		return err
	}
	originalAttributes, originalKnown := original.Sys().(*sftp.FileStat)
	currentAttributes, currentKnown := current.Sys().(*sftp.FileStat)
	if originalKnown && currentKnown && (originalAttributes.UID != currentAttributes.UID || originalAttributes.GID != currentAttributes.GID) {
		if err := remote.client.Chown(filename, int(originalAttributes.UID), int(originalAttributes.GID)); err != nil {
			return fault.New(409, "file_owner_not_preserved", "无法保留原文件的属主或组，已停止保存。")
		}
	}
	return nil
}
func (remote *remoteFiles) Close() error {
	remote.once.Do(func() {
		remote.client.Close()
		remote.release()
	})
	return nil
}
