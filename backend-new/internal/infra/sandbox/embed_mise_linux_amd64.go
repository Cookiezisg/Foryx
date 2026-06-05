//go:build linux && amd64

package sandbox

import _ "embed"

//go:embed mise/linux-amd64/mise
var miseBinary []byte
