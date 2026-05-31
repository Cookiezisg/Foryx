package flowrun

import "context"

// JournalRepository is the append-only journal port (17 §2). AppendEvent allocates seq inside
// the write tx and inserts; a record-once collision (dedup_key) is a no-op that returns the
// already-recorded event (compare-and-insert / first-wins, ADR-018). LoadJournal returns the
// flowrun's events in seq order — the input to deterministic replay.
//
// JournalRepository 是 append-only journal port;撞 record-once 键 = no-op 返既有事件(first-wins)。
type JournalRepository interface {
	AppendEvent(ctx context.Context, e *FlowRunEvent) (*FlowRunEvent, error)
	LoadJournal(ctx context.Context, flowrunID string) ([]FlowRunEvent, error)
}
