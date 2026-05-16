//go:build darwin && arm64

package sandbox

import _ "embed"

//go:embed mise/darwin-arm64/mise
var miseBinary []byte
