package chat

import (
	"encoding/base64"
	"fmt"
	"os"

	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
)

func newMsgID() string        { return idgenpkg.New("msg") }
func newBlockID() string      { return idgenpkg.New("blk") }
func newAttachmentID() string { return idgenpkg.New("att") }

func encodeBase64(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// readAndEncode reads a file from disk and returns its base64-encoded content.
//
// readAndEncode 从磁盘读取文件并返回其 base64 编码内容。
func readAndEncode(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("readAndEncode: %w", err)
	}
	return encodeBase64(data), nil
}
