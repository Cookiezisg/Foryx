package handlers

import (
	"net/http"
	"runtime"
	"time"

	responsehttpapi "github.com/sunweilin/forgify/backend/internal/transport/httpapi/response"
)

// Runtime returns a single-shot Go-runtime snapshot for the Metrics tab.
//
// Runtime 单次快照 Go runtime 状态,对应 Metrics tab。
func (h *DevHandler) Runtime(w http.ResponseWriter, r *http.Request) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	uptimeSeconds := int64(0)
	if h.startedAt != (time.Time{}) {
		uptimeSeconds = int64(time.Since(h.startedAt).Seconds())
	}

	out := map[string]any{
		"goVersion":      runtime.Version(),
		"goroutines":     runtime.NumGoroutine(),
		"cgoCalls":       runtime.NumCgoCall(),
		"numCPU":         runtime.NumCPU(),
		"maxProcs":       runtime.GOMAXPROCS(0),
		"uptimeSeconds":  uptimeSeconds,
		"mem": map[string]any{
			"heapAlloc":  ms.HeapAlloc,
			"heapSys":    ms.HeapSys,
			"heapInuse":  ms.HeapInuse,
			"heapIdle":   ms.HeapIdle,
			"stackInuse": ms.StackInuse,
			"sys":        ms.Sys,
		},
		"gc": map[string]any{
			"numGC":       ms.NumGC,
			"numForcedGC": ms.NumForcedGC,
			"lastPauseNs": ms.PauseNs[(ms.NumGC+255)%256],
			"totalPauseNs": ms.PauseTotalNs,
			"cpuFraction":  ms.GCCPUFraction,
		},
	}

	if sqlDB, err := h.db.DB(); err == nil {
		s := sqlDB.Stats()
		out["db"] = map[string]any{
			"openConnections":    s.OpenConnections,
			"inUse":              s.InUse,
			"idle":               s.Idle,
			"waitCount":          s.WaitCount,
			"waitDurationNs":     s.WaitDuration.Nanoseconds(),
			"maxOpenConnections": s.MaxOpenConnections,
		}
	}

	responsehttpapi.Success(w, http.StatusOK, out)
}
