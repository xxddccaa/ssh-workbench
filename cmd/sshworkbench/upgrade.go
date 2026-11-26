package main

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const defaultRepo = "xxddccaa/ssh-workbench"

func upgradeCommand(arguments []string) error {
	flags := flag.NewFlagSet("upgrade", flag.ContinueOnError)
	repo := flags.String("repo", defaultRepo, "GitHub repository (owner/name)")
	targetVersion := flags.String("version", "", "target release tag (default: latest)")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	client := &http.Client{Timeout: 60 * time.Second}
	tag := strings.TrimSpace(*targetVersion)
	if tag == "" {
		resolved, err := latestReleaseTag(client, *repo)
		if err != nil {
			return err
		}
		tag = resolved
	}
	ver := strings.TrimPrefix(tag, "v")
	if ver == strings.TrimPrefix(version, "v") && *targetVersion == "" {
		fmt.Printf("已是最新版本：%s\n", version)
		return nil
	}
	ext := "tar.gz"
	if runtime.GOOS == "windows" {
		ext = "zip"
	}
	archiveName := fmt.Sprintf("sshworkbench_%s_%s_%s.%s", ver, runtime.GOOS, runtime.GOARCH, ext)
	baseURL := fmt.Sprintf("https://github.com/%s/releases/download/%s", *repo, tag)
	fmt.Printf("正在下载 %s (%s)...\n", archiveName, tag)
	archiveBytes, err := downloadBytes(client, baseURL+"/"+archiveName, 128<<20)
	if err != nil {
		return fmt.Errorf("download %s: %w", archiveName, err)
	}
	if sumsBytes, err := downloadBytes(client, baseURL+"/SHA256SUMS", 1<<20); err == nil {
		if expected := findChecksum(sumsBytes, archiveName); expected != "" {
			actual := sha256Hex(archiveBytes)
			if !strings.EqualFold(actual, expected) {
				return fmt.Errorf("SHA-256 校验失败: expected %s, got %s", expected, actual)
			}
		}
	}
	binaryName := "sshworkbench"
	if runtime.GOOS == "windows" {
		binaryName = "sshworkbench.exe"
	}
	binaryData, err := extractBinary(archiveBytes, ext, binaryName)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(executable); err == nil {
		executable = resolved
	}
	if err := replaceBinary(executable, binaryData); err != nil {
		return err
	}
	fmt.Printf("升级完成：%s -> %s (%s)\n", version, ver, executable)
	return nil
}

func latestReleaseTag(client *http.Client, repo string) (string, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "sshworkbench/"+version)
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %s", response.Status)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return "", err
	}
	if payload.TagName == "" {
		return "", fmt.Errorf("latest release tag not found")
	}
	return payload.TagName, nil
}

func downloadBytes(client *http.Client, url string, limit int64) ([]byte, error) {
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "sshworkbench/"+version)
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %s", response.Status)
	}
	return io.ReadAll(io.LimitReader(response.Body, limit))
}

func findChecksum(sums []byte, filename string) string {
	scanner := bufio.NewScanner(bytes.NewReader(sums))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && filepath.Base(fields[1]) == filename {
			return fields[0]
		}
	}
	return ""
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func extractBinary(archiveBytes []byte, ext, binaryName string) ([]byte, error) {
	if ext == "zip" {
		reader, err := zip.NewReader(bytes.NewReader(archiveBytes), int64(len(archiveBytes)))
		if err != nil {
			return nil, err
		}
		for _, file := range reader.File {
			if filepath.Base(file.Name) == binaryName && !file.FileInfo().IsDir() {
				rc, err := file.Open()
				if err != nil {
					return nil, err
				}
				defer rc.Close()
				return io.ReadAll(io.LimitReader(rc, 128<<20))
			}
		}
		return nil, fmt.Errorf("%s not found in zip archive", binaryName)
	}
	gz, err := gzip.NewReader(bytes.NewReader(archiveBytes))
	if err != nil {
		return nil, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag == tar.TypeReg && filepath.Base(header.Name) == binaryName {
			return io.ReadAll(io.LimitReader(tr, 128<<20))
		}
	}
	return nil, fmt.Errorf("%s not found in tar.gz archive", binaryName)
}

func replaceBinary(target string, data []byte) error {
	dir := filepath.Dir(target)
	tmp, err := os.CreateTemp(dir, ".sshworkbench-upgrade-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			os.Remove(tmpPath)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		backup := target + ".old"
		_ = os.Remove(backup)
		if err := os.Rename(target, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return err
	}
	cleanup = false
	return nil
}
