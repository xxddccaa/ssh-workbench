package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"testing"
)

func TestFindChecksumAndExtractBinary(t *testing.T) {
	sums := []byte("abcd1234  sshw_0.3.0_darwin_arm64.tar.gz\n9999eeee  sshw_0.3.0_windows_amd64.zip\n")
	if got := findChecksum(sums, "sshw_0.3.0_darwin_arm64.tar.gz"); got != "abcd1234" {
		t.Fatalf("findChecksum tar.gz = %q", got)
	}
	if got := findChecksum(sums, "missing.tar.gz"); got != "" {
		t.Fatalf("expected empty checksum for missing file, got %q", got)
	}

	var tarBuf bytes.Buffer
	gw := gzip.NewWriter(&tarBuf)
	tw := tar.NewWriter(gw)
	payload := []byte("fake-binary")
	if err := tw.WriteHeader(&tar.Header{Name: "sshw_0.3.0_darwin_arm64/sshw", Mode: 0o755, Size: int64(len(payload))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(payload); err != nil {
		t.Fatal(err)
	}
	tw.Close()
	gw.Close()
	got, err := extractBinary(tarBuf.Bytes(), "tar.gz", "sshw")
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("extract tar.gz = %q, %v", got, err)
	}

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	fw, err := zw.Create("sshw_0.3.0_windows_amd64/sshw.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(payload); err != nil {
		t.Fatal(err)
	}
	zw.Close()
	gotZip, err := extractBinary(zipBuf.Bytes(), "zip", "sshw.exe")
	if err != nil || !bytes.Equal(gotZip, payload) {
		t.Fatalf("extract zip = %q, %v", gotZip, err)
	}
}
