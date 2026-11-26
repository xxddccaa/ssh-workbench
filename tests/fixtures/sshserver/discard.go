package sshserver

import (
	"io"
	"os"
	"strings"

	"github.com/pkg/sftp"
)

// DiscardDirectory accepts uploads and drops their bytes. pkg/sftp's
// in-memory handler deliberately sleeps one microsecond per written byte and
// keeps everything in RAM, which makes it unusable for measuring large
// transfers; uploads below this directory measure only the client path.
const DiscardDirectory = "/discard"

type discardWriter struct{}

func (discardWriter) WriteAt(data []byte, _ int64) (int, error) { return len(data), nil }

type discardingWriters struct {
	next sftp.FileWriter
}

func (writers discardingWriters) Filewrite(request *sftp.Request) (io.WriterAt, error) {
	if strings.HasPrefix(request.Filepath, DiscardDirectory+"/") {
		return discardWriter{}, nil
	}
	return writers.next.Filewrite(request)
}

// directorySetstat makes SETSTAT on directories succeed as a no-op, the same
// way pkg/sftp's in-memory handler already ignores mode bits on files (it
// would otherwise try to open the directory as a file and fail).
type directorySetstat struct {
	next   sftp.FileCmder
	lister sftp.FileLister
}

func (commands directorySetstat) Filecmd(request *sftp.Request) error {
	if request.Method == "Setstat" {
		if listing, err := commands.lister.Filelist(sftp.NewRequest("Stat", request.Filepath)); err == nil {
			items := make([]os.FileInfo, 1)
			if count, _ := listing.ListAt(items, 0); count == 1 && items[0].IsDir() {
				return nil
			}
		}
	}
	return commands.next.Filecmd(request)
}

// PosixRename keeps the wrapped handler's posix-rename@openssh.com support;
// without it the request server falls back to a rename that never replaces.
func (commands directorySetstat) PosixRename(request *sftp.Request) error {
	if renamer, ok := commands.next.(sftp.PosixRenameFileCmder); ok {
		return renamer.PosixRename(request)
	}
	return commands.next.Filecmd(request)
}

func withDiscard(handlers sftp.Handlers) sftp.Handlers {
	handlers.FilePut = discardingWriters{next: handlers.FilePut}
	handlers.FileCmd = directorySetstat{next: handlers.FileCmd, lister: handlers.FileList}
	return handlers
}
