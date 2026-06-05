//go:build darwin && amd64

package sandbox

import _ "embed"

//go:embed mise/darwin-amd64/mise
var miseBinary []byte
