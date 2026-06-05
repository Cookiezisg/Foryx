//go:build linux && arm64

package sandbox

import _ "embed"

//go:embed mise/linux-arm64/mise
var miseBinary []byte
