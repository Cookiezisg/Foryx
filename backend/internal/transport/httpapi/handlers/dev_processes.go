package handlers

import (
	"net/http"
	"strconv"

	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// BashProcesses returns Bash tool's background-process registry; sample = tail bytes.
//
// BashProcesses 返 Bash 工具后台进程注册表;sample = 尾字节数。
func (h *DevHandler) BashProcesses(w http.ResponseWriter, r *http.Request) {
	sample := 2048
	if s := r.URL.Query().Get("sample"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			sample = n
		}
	}
	if sample > 16384 {
		sample = 16384
	}
	snaps := h.shellManager.Snapshots(sample)
	responsehttpapi.Success(w, http.StatusOK, map[string]any{
		"count":     len(snaps),
		"sample":    sample,
		"processes": snaps,
	})
}
