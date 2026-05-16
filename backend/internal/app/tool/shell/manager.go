package shell

import (
	"errors"
	"os/exec"
	"sync"
	"time"

	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
)

// bgBufferBytes caps the per-process ring buffer; oldest bytes drop on overflow.
//
// bgBufferBytes 限制单进程环形缓冲；溢出丢最旧字节。
const bgBufferBytes = 256 * 1024

// Status reports a background process's current lifecycle phase.
//
// Status 报告后台进程的生命周期阶段。
type Status string

const (
	StatusRunning Status = "running"
	StatusExited  Status = "exited"
	StatusKilled  Status = "killed"
	StatusErrored Status = "errored"
)

var (
	// ErrProcessNotFound: bash_id unknown.
	//
	// ErrProcessNotFound：bash_id 未知。
	ErrProcessNotFound = errors.New("background shell process not found")
)

// BgProcess holds one tracked child; output buffer + cursor are guarded by mu.
//
// BgProcess 是一个被追踪的子进程；输出缓冲与游标受 mu 保护。
type BgProcess struct {
	ID        string
	ConvID    string
	Command   string
	Cmd       *exec.Cmd
	StartedAt time.Time

	mu         sync.Mutex
	buf        []byte
	dropped    int64
	readCursor int
	status     Status
	exitCode   int
	finishedAt time.Time
	launchErr  error
}

// appendOutput appends b to the ring buffer; on overflow drops from the front and rewinds the cursor.
//
// appendOutput 把 b 追加到环形缓冲；溢出时从头丢并相应回退游标。
func (p *BgProcess) appendOutput(b []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.buf = append(p.buf, b...)
	if len(p.buf) <= bgBufferBytes {
		return
	}
	overflow := len(p.buf) - bgBufferBytes
	p.dropped += int64(overflow)
	p.buf = p.buf[overflow:]
	p.readCursor -= overflow
	if p.readCursor < 0 {
		p.readCursor = 0
	}
}

// drainNew returns bytes appended since last drain and advances the cursor.
//
// drainNew 返回上次以来追加的字节并推进游标。
func (p *BgProcess) drainNew() (newBytes []byte, dropped int64, status Status, exitCode int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := append([]byte(nil), p.buf[p.readCursor:]...)
	p.readCursor = len(p.buf)
	return out, p.dropped, p.status, p.exitCode
}

func (p *BgProcess) markFinished(status Status, exitCode int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = status
	p.exitCode = exitCode
	p.finishedAt = time.Now()
}

func (p *BgProcess) markErrored(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = StatusErrored
	p.launchErr = err
	p.finishedAt = time.Now()
}

// ProcessManager owns the registry of background shell processes.
//
// ProcessManager 持有后台 shell 进程的注册表。
type ProcessManager struct {
	mu    sync.Mutex
	procs map[string]*BgProcess
}

// NewProcessManager returns an empty manager.
//
// NewProcessManager 返一个空 manager。
func NewProcessManager() *ProcessManager {
	return &ProcessManager{procs: make(map[string]*BgProcess)}
}

// Register stamps an ID and stores the process; caller must have set command + Cmd before calling.
//
// Register 派 ID 并入库；调用方须已填好 Command + Cmd。
func (m *ProcessManager) Register(p *BgProcess) {
	if p.ID == "" {
		p.ID = idgenpkg.New("bsh")
	}
	if p.StartedAt.IsZero() {
		p.StartedAt = time.Now()
	}
	if p.status == "" {
		p.status = StatusRunning
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.procs[p.ID] = p
}

// Get returns the process by ID or ErrProcessNotFound.
//
// Get 按 ID 返进程，找不到返 ErrProcessNotFound。
func (m *ProcessManager) Get(id string) (*BgProcess, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.procs[id]
	if !ok {
		return nil, ErrProcessNotFound
	}
	return p, nil
}

// Remove drops the entry; used by KillShell after killing + reaping.
//
// Remove 删除注册表条目；KillShell 杀完 reap 后调用。
func (m *ProcessManager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.procs, id)
}

// Snapshot is a non-mutating view of one BgProcess for /dev/bash-processes; Sample does not advance the read cursor.
//
// Snapshot 是单进程的只读快照；Sample 不动 BashOutput 游标。
type Snapshot struct {
	ID         string    `json:"id"`
	ConvID     string    `json:"convId,omitempty"`
	Command    string    `json:"command"`
	Status     Status    `json:"status"`
	ExitCode   int       `json:"exitCode"`
	StartedAt  time.Time `json:"startedAt"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
	BufLen     int       `json:"bufLen"`
	Dropped    int64     `json:"dropped"`
	ReadCursor int       `json:"readCursor"`
	Sample     string    `json:"sample,omitempty"`
	LaunchErr  string    `json:"launchErr,omitempty"`
}

func (p *BgProcess) snapshot(sampleBytes int) Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := Snapshot{
		ID:         p.ID,
		ConvID:     p.ConvID,
		Command:    p.Command,
		Status:     p.status,
		ExitCode:   p.exitCode,
		StartedAt:  p.StartedAt,
		FinishedAt: p.finishedAt,
		BufLen:     len(p.buf),
		Dropped:    p.dropped,
		ReadCursor: p.readCursor,
	}
	if p.launchErr != nil {
		s.LaunchErr = p.launchErr.Error()
	}
	if sampleBytes > 0 && len(p.buf) > 0 {
		start := 0
		if len(p.buf) > sampleBytes {
			start = len(p.buf) - sampleBytes
		}
		s.Sample = string(p.buf[start:])
	}
	return s
}

// Snapshots returns snapshots of every tracked process, newest first.
//
// Snapshots 返每个追踪进程的快照，最新优先。
func (m *ProcessManager) Snapshots(sampleBytes int) []Snapshot {
	m.mu.Lock()
	procs := make([]*BgProcess, 0, len(m.procs))
	for _, p := range m.procs {
		procs = append(procs, p)
	}
	m.mu.Unlock()
	out := make([]Snapshot, 0, len(procs))
	for _, p := range procs {
		out = append(out, p.snapshot(sampleBytes))
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].StartedAt.After(out[j-1].StartedAt); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// Stop best-effort kills every running child during graceful shutdown.
//
// Stop 优雅关停时尽力杀掉所有 running 子进程。
func (m *ProcessManager) Stop() {
	m.mu.Lock()
	procs := make([]*BgProcess, 0, len(m.procs))
	for _, p := range m.procs {
		procs = append(procs, p)
	}
	m.mu.Unlock()
	for _, p := range procs {
		if p.Cmd == nil || p.Cmd.Process == nil {
			continue
		}
		_ = p.Cmd.Process.Kill()
	}
}
