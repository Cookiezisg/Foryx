//go:build windows && amd64

package sandbox

import _ "embed"

//go:embed mise/windows-amd64/mise.exe
var miseBinary []byte
