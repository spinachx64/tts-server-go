//go:build (linux && !android && (386 || amd64 || arm || arm64 || loong64 || ppc64le || riscv64 || s390x)) || (darwin && (amd64 || arm64)) || (windows && (386 || amd64 || arm64)) || (freebsd && (386 || amd64 || arm || arm64)) || (netbsd && amd64) || (openbsd && (amd64 || arm64))

package tokenstore

import _ "modernc.org/sqlite"
