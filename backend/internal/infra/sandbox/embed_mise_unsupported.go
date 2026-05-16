//go:build !((darwin && (arm64 || amd64)) || (linux && (amd64 || arm64)) || (windows && amd64))

package sandbox

var miseBinary []byte
