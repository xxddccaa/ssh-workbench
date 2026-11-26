package localdocs

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"sshworkbench/internal/fault"
)

func code(err error) string {
	var problem *fault.Error
	if errors.As(err, &problem) {
		return problem.Code
	}
	return ""
}

func TestLocalDocumentKeepsFormatAndDetectsConflicts(test *testing.T) {
	directory := test.TempDir()
	filename := filepath.Join(directory, "配置.txt")
	if err := os.WriteFile(filename, []byte("\ufeff第一行\r\n第二行\r\n"), 0640); err != nil {
		test.Fatal(err)
	}
	service := New()
	if _, err := service.Register("relative.txt"); code(err) != "absolute_path_required" {
		test.Fatal("relative path accepted")
	}
	registration, err := service.Register(filename)
	if err != nil {
		test.Fatal(err)
	}
	document, err := service.Read(registration.ID)
	if err != nil || !document.BOM || document.LineEnding != "crlf" {
		test.Fatalf("format not detected: %+v %v", document, err)
	}
	document.Content = "修改\n"
	saved, err := service.Save(registration.ID, document)
	if err != nil {
		test.Fatal(err)
	}
	data, _ := os.ReadFile(filename)
	if string(data) != "\ufeff修改\r\n" || saved.Version == document.Version {
		test.Fatalf("format not preserved: %q", data)
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(filename); info.Mode().Perm() != 0640 {
			test.Fatalf("permissions changed: %v", info.Mode())
		}
	}
	if err := os.WriteFile(filename, []byte("external\n"), 0640); err != nil {
		test.Fatal(err)
	}
	saved.Content = "lost update\n"
	if _, err := service.Save(registration.ID, saved); code(err) != "document_conflict" {
		test.Fatalf("conflict not detected: %v", err)
	}
	if entries, _ := os.ReadDir(directory); len(entries) != 1 {
		test.Fatalf("temporary files left behind: %v", entries)
	}
	if _, err := service.Read("unknown"); code(err) != "local_document_not_found" {
		test.Fatal("unregistered document readable")
	}
}

func TestLocalDocumentRejectsSpecialAndLargeFiles(test *testing.T) {
	directory := test.TempDir()
	service := New()
	if _, err := service.Register(directory); code(err) != "not_editable" {
		test.Fatal("directory registered")
	}
	large := filepath.Join(directory, "large.log")
	if err := os.WriteFile(large, make([]byte, 3*1024*1024), 0600); err != nil {
		test.Fatal(err)
	}
	if _, err := service.Register(large); code(err) != "not_editable" {
		test.Fatal("large file registered")
	}
	binary := filepath.Join(directory, "binary.dat")
	os.WriteFile(binary, []byte{0xff, 0x00, 0x01}, 0600)
	registration, err := service.Register(binary)
	if err != nil {
		test.Fatal(err)
	}
	if _, err := service.Read(registration.ID); code(err) != "unsupported_encoding" {
		test.Fatal("binary file decoded")
	}
}
