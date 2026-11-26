#!/bin/sh
set -eu

project_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$project_root"
version="${VERSION:-$(sed -n 's/^var version = "\(.*\)"/\1/p' cmd/sshw/main.go)}"
version="${version#v}"
output="dist/$version"
rm -rf "$output"
mkdir -p "$output"
for target in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64 windows/amd64; do
  goos=${target%/*}
  goarch=${target#*/}
  name="sshw_${version}_${goos}_${goarch}"
  staging="$output/$name"
  mkdir -p "$staging"
  binary=sshw
  [ "$goos" = windows ] && binary=sshw.exe
  CGO_ENABLED=0 GOOS=$goos GOARCH=$goarch go build -trimpath -ldflags="-s -w -X main.version=${version}" -o "$staging/$binary" ./cmd/sshw
  cp README.md THIRD_PARTY_NOTICES.md "$staging/"
  mkdir -p "$staging/docs"
  cp docs/*.md "$staging/docs/"
  if [ "$goos" = windows ]; then
    (cd "$output" && zip -qr "$name.zip" "$name")
  else
    tar -C "$output" -czf "$output/$name.tar.gz" "$name"
  fi
  rm -rf "$staging"
done
(cd "$output" && shasum -a 256 ./*.tar.gz ./*.zip > SHA256SUMS)
echo "Archives and SHA256SUMS written to $output"
