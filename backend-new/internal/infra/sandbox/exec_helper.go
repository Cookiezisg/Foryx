package sandbox

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"

	sandboxdomain "github.com/sunweilin/forgify/backend/internal/domain/sandbox"
)

const stderrTailMax = 4096

// RunWithStderrCapture runs cmd, fans stderr to stream and tail buffer; wraps non-zero exit.
//
// RunWithStderrCapture 运行 cmd，stderr 同时进 stream 与 tail；非零退出包装错误。
func RunWithStderrCapture(cmd *exec.Cmd, stream sandboxdomain.ProgressFunc, sentinel error, msgPrefix string) error {
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("%s: stderr pipe: %w", msgPrefix, err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: start: %w", msgPrefix, err)
	}

	var tail []byte
	scanner := bufio.NewScanner(stderrPipe)
	for scanner.Scan() {
		line := scanner.Text()
		if stream != nil {
			stream("running", line, -1)
		}
		tail = append(tail, line...)
		tail = append(tail, '\n')
		if len(tail) > stderrTailMax {
			tail = tail[len(tail)-stderrTailMax:]
		}
	}
	if scanErr := scanner.Err(); scanErr != nil {
		tail = append(tail, []byte("(stderr scan error: "+scanErr.Error()+")\n")...)
	}

	if err := cmd.Wait(); err != nil {
		snippet := strings.TrimSpace(string(tail))
		if snippet == "" {
			snippet = "(no stderr)"
		}
		return fmt.Errorf("%s: %w: %w: %s", msgPrefix, sentinel, err, snippet)
	}
	return nil
}

var _ = sandboxdomain.ErrRuntimeInstallFailed
