#!/bin/bash
set -e

VERSION=${1:-1.2}

echo "Building utsms-daemon version $VERSION ..."

# 1) Build Go binary
go build -o utsms-daemon main.go

# 2) Copy binary into package tree
cp utsms-daemon debian/usr/bin/

# 3) Build .deb package
DEB_NAME="utsms-daemon_${VERSION}_amd64.deb"
dpkg-deb --build debian "$DEB_NAME"

echo "Done: $DEB_NAME"
