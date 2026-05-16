package mcp

import "os"

func defaultOSEnviron() []string {
	return os.Environ()
}
