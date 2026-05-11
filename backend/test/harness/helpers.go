//go:build pipeline

// helpers.go — shared HTTP, assertion and DB helpers used across all pipeline
// test files. Keep test bodies focused on business assertions; put plumbing here.
//
// helpers.go — 所有 pipeline 测试共用的 HTTP、断言和 DB helper。
// 让 test body 专注业务断言，样板代码放这里。
package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"testing"

	chatdomain "github.com/sunweilin/forgify/backend/internal/domain/chat"
	eventlogdomain "github.com/sunweilin/forgify/backend/internal/domain/eventlog"
	reqctxpkg "github.com/sunweilin/forgify/backend/internal/pkg/reqctx"
)

// ErrEnvelope decodes the standard error envelope shape.
//
// ErrEnvelope 解标准错误 envelope 形状。
type ErrEnvelope struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// LocalCtxAs returns a context stamped with the given userID, suitable for
// service-layer calls that need a specific user identity (e.g. cross-user
// isolation tests that cannot rely on the hardcoded InjectUserID middleware).
//
// LocalCtxAs 返回打了指定 userID 的 ctx，用于需要特定用户身份的 service 层调用
// （如跨用户隔离测试，无法依赖硬编码的 InjectUserID 中间件）。
func LocalCtxAs(userID string) context.Context {
	return reqctxpkg.SetUserID(context.Background(), userID)
}

// PostMessage POSTs a user message to a conversation and returns the allocated
// user message ID. Fails the test on transport error or empty response.
//
// PostMessage 向对话 POST 一条用户消息，返回分配的 user message ID。
// 传输错误或响应为空时 fail。
func PostMessage(t *testing.T, h *Harness, convID, content string) string {
	t.Helper()
	var resp struct {
		Data struct {
			MessageID string `json:"messageId"`
		} `json:"data"`
	}
	h.PostJSON("/api/v1/conversations/"+convID+"/messages",
		map[string]any{"content": content}, &resp)
	if resp.Data.MessageID == "" {
		t.Fatalf("PostMessage: empty messageId in response")
	}
	return resp.Data.MessageID
}

// PostFunction is a shorthand for POST /api/v1/functions.
//
// PostFunction 是 POST /api/v1/functions 的简写。
func PostFunction(t *testing.T, h *Harness, name, code string, out any) int {
	t.Helper()
	return DoRequest(t, h, "POST", "/api/v1/functions", map[string]any{
		"name": name,
		"code": code,
	}, out)
}

// ExtractTextFromBlocks concatenates the content of all text blocks in order.
//
// ExtractTextFromBlocks 按顺序拼接所有 text block 的内容。
func ExtractTextFromBlocks(blocks []chatdomain.Block) string {
	var b strings.Builder
	for _, blk := range blocks {
		if blk.Type != eventlogdomain.BlockTypeText {
			continue
		}
		b.WriteString(blk.Content)
	}
	return b.String()
}

// ExtractToolCallByName finds the first tool_call block matching name and
// returns its call ID. The block ID is the LLM-assigned tool-call ID.
//
// ExtractToolCallByName 找第一个匹配 name 的 tool_call block，返回 call ID。
// block ID 即 LLM 分配的 tool-call ID。
func ExtractToolCallByName(blocks []chatdomain.Block, name string) (id string, found bool) {
	for _, blk := range blocks {
		if blk.Type != eventlogdomain.BlockTypeToolCall {
			continue
		}
		var attrs map[string]any
		if err := json.Unmarshal([]byte(blk.Attrs), &attrs); err != nil {
			continue
		}
		if n, _ := attrs["tool"].(string); n == name {
			return blk.ID, true
		}
	}
	return "", false
}

// ExtractToolResultByCallID finds the tool_result block paired with callID
// (parent_block_id == callID) and returns a synthesized envelope that
// matches what older tests expected:
//
//	data["ok"]       = block.Status == "completed"
//	data["result"]   = block.Content (the raw tool output string)
//	data["error"]    = block.Error (only when status=error)
//
// Tools that return a JSON object as their raw output keep their fields
// inside data["result"]; callers can json.Unmarshal that string when they
// need to inspect specific fields.
//
// ExtractToolResultByCallID 找与 callID 配对的 tool_result block
// （parent_block_id == callID），返回合成 envelope（与老测试期望一致）。
// 返 JSON 对象的 tool，其原始输出整字符串放在 data["result"]，需要时调用
// 方再 json.Unmarshal。
func ExtractToolResultByCallID(blocks []chatdomain.Block, callID string) (data map[string]any, found bool) {
	for _, blk := range blocks {
		if blk.Type != eventlogdomain.BlockTypeToolResult {
			continue
		}
		if blk.ParentBlockID != callID {
			continue
		}
		out := map[string]any{
			"ok":     blk.Status == eventlogdomain.StatusCompleted,
			"result": blk.Content,
		}
		if blk.Error != "" {
			out["error"] = blk.Error
		}
		return out, true
	}
	return nil, false
}

// DoRequest sends a JSON request and returns the HTTP status code WITHOUT
// failing the test on non-2xx responses. If out is non-nil, the response body
// is decoded into it. Use this when the test expects an error status code.
//
// DoRequest 发送 JSON 请求，返回 HTTP 状态码，非 2xx 不 fail 测试。
// out 非 nil 时解码响应体。用于期望错误状态的测试。
func DoRequest(t *testing.T, h *Harness, method, path string, body, out any) int {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("DoRequest: marshal body: %v", err)
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequest(method, h.URL()+path, rdr)
	if err != nil {
		t.Fatalf("DoRequest: build request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := h.HTTPClient().Do(req)
	if err != nil {
		t.Fatalf("DoRequest: %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	if out != nil {
		raw, _ := io.ReadAll(resp.Body)
		if err := json.Unmarshal(raw, out); err != nil {
			t.Logf("DoRequest: decode response (status=%d): %v; body=%s", resp.StatusCode, err, raw)
		}
	}
	return resp.StatusCode
}

// UploadFile uploads data as a multipart file to POST /api/v1/attachments.
// Returns the allocated attachment ID. Fails the test on error or non-201.
//
// UploadFile 把 data 作为 multipart file POST 到 /api/v1/attachments，
// 返回分配的 attachment ID。失败或非 201 则 fail。
func UploadFile(t *testing.T, h *Harness, filename, mimeType string, data []byte) string {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	header.Set("Content-Type", mimeType)
	part, err := mw.CreatePart(header)
	if err != nil {
		t.Fatalf("UploadFile: create part: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("UploadFile: write data: %v", err)
	}
	mw.Close()

	req, err := http.NewRequest("POST", h.URL()+"/api/v1/attachments", &body)
	if err != nil {
		t.Fatalf("UploadFile: build request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := h.HTTPClient().Do(req)
	if err != nil {
		t.Fatalf("UploadFile: do: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("UploadFile: status=%d, want 201; body=%s", resp.StatusCode, raw)
	}
	var out struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.Data.ID == "" {
		t.Fatalf("UploadFile: decode response: %v; body=%s", err, raw)
	}
	return out.Data.ID
}

// DBCount returns the row count for table matching the optional WHERE clause.
// Fails the test on query error.
//
// DBCount 返回 table 中匹配可选 WHERE 条件的行数，查询出错则 fail。
func DBCount(t *testing.T, h *Harness, table, where string, args ...any) int64 {
	t.Helper()
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
	if where != "" {
		query += " WHERE " + where
	}
	var count int64
	if err := h.DB.Raw(query, args...).Scan(&count).Error; err != nil {
		t.Fatalf("DBCount %s: %v", table, err)
	}
	return count
}
