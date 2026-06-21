package scheduler

import (
	"context"
	"time"
)

// advanceWorkers is the worker count for the async Advance pool. SMALL on purpose (F174): the DB is a
// single connection (SetMaxOpenConns(1), db.go) so all durable writes serialize Go-side regardless,
// and a handler's resident instance is a single mutexed stdio pipe — so the only thing extra workers
// parallelize is the I/O-bound slow call (a function sandbox, an agent LLM turn, an MCP request). A
// small N captures essentially all of that benefit while BOUNDING concurrent subprocess fan-out on a
// single-user desktop (the systems-correctness R-series concern). Not a settings knob — the ceiling is
// structural, not user-tunable.
//
// advanceWorkers 是异步 Advance 池的 worker 数。**刻意小**（F174）：DB 单连接（SetMaxOpenConns(1)）故
// 所有 durable 写在 Go 层本就串行，handler 常驻实例是单 mutex stdio 管道——故多 worker 唯一并行的是
// I/O 密集的慢调用（function sandbox / agent LLM turn / MCP 请求）。小 N 吃满几乎全部收益、同时**封顶**
// 单用户桌面的并发子进程扇出（R 系列系统正确性顾虑）。非 settings 旋钮——天花板是结构性的、非用户可调。
const advanceWorkers = 4

// advanceQueueDepth buffers enqueued advances. The per-run dedup (advQueued) keeps at most one job per
// distinct running run in flight, so this never fills on a single-user box; a generous buffer lets the
// drain goroutine enqueue a whole 100-firing batch + boot Recover without ever blocking.
//
// advanceQueueDepth 缓冲入队的 advance。per-run 去重（advQueued）保证每个 distinct running run 至多一条
// 在途，故单用户机永不填满；宽缓冲让 drain goroutine 入队整批 100 firing + boot Recover 而绝不阻塞。
const advanceQueueDepth = 512

// advanceJob is one enqueued advance: drive runID under ctx (a Detached, workspace-scoped ctx).
//
// advanceJob 是一条入队的 advance：在 ctx（Detached、workspace 作用域）下驱动 runID。
type advanceJob struct {
	ctx   context.Context
	runID string
}

// StartPool spawns the Advance worker pool. Call ONCE at boot, BEFORE Recover, so Recover's enqueued
// runs resume on the pool (off the boot goroutine) the way a slow node should never block boot (the
// F174 boot variant). After this, enqueueAdvance buffers + dispatches to workers; before it (tests /
// manual-only), enqueueAdvance drives inline.
//
// StartPool 启动 Advance worker 池。boot 时调一次、在 Recover **之前**，使 Recover 入队的 run 在池上恢复
// （脱离 boot goroutine）——慢节点不该卡 boot（F174 的 boot 变体）。此后 enqueueAdvance 缓冲 + 派发给
// worker；此前（测试/纯手动）enqueueAdvance 内联驱动。
func (s *Service) StartPool() {
	s.advMu.Lock()
	if s.advStarted {
		s.advMu.Unlock()
		return
	}
	s.advQueue = make(chan advanceJob, advanceQueueDepth)
	s.advStarted = true
	q := s.advQueue
	s.advMu.Unlock()
	for i := 0; i < advanceWorkers; i++ {
		s.advWG.Add(1)
		go func() {
			defer s.advWG.Done()
			for job := range q {
				s.drive(job.ctx, job.runID)
			}
		}()
	}
}

// StopPool stops accepting new enqueues, then waits for all workers to drain the queue and exit (so a
// pool worker can never write to a closing DB — Shutdown cancels in-flight ctx first, then StopPool
// waits). Idempotent. After StopPool, enqueueAdvance falls back to inline drive (harmless — such a late
// drive registers an in-flight ctx that Shutdown's cancel-all already covers).
//
// StopPool 停止接受新入队，再等所有 worker 排空队列并退出（故 pool worker 绝不写正在关闭的 DB——Shutdown
// 先 cancel 在飞 ctx、StopPool 再等）。幂等。StopPool 后 enqueueAdvance 回退内联驱动（无害——此种迟到驱动
// 注册的在飞 ctx 已被 Shutdown 的 cancel-all 覆盖）。
func (s *Service) StopPool() {
	s.advMu.Lock()
	if !s.advStarted {
		s.advMu.Unlock()
		return
	}
	s.advStarted = false
	q := s.advQueue
	s.advQueue = nil
	s.advMu.Unlock()
	close(q)
	s.advWG.Wait()
}

// WaitPoolDrained blocks until the pool is idle (no run in progress and none queued) or the grace
// window / ctx expires. Used in Shutdown to give in-flight nodes a bounded chance to finish cleanly
// (record-once → clean resume next boot) BEFORE cancel-all hard-interrupts them. Polls — a shutdown
// path, not a hot path; avoids a leaked cond-wait goroutine.
//
// WaitPoolDrained 阻塞到池空闲（无 run 在驱、无 run 在队）或宽限/ctx 到期。Shutdown 用它给在飞节点有界
// 机会干净跑完（record-once → 下次 boot 干净续）再 cancel-all 硬打断。轮询——关停路径非热路径，免泄漏
// cond-wait goroutine。
func (s *Service) WaitPoolDrained(ctx context.Context, grace time.Duration) {
	deadline := time.NewTimer(grace)
	defer deadline.Stop()
	tick := time.NewTicker(20 * time.Millisecond)
	defer tick.Stop()
	for {
		s.advMu.Lock()
		idle := len(s.advInProgress) == 0 && len(s.advQueued) == 0
		s.advMu.Unlock()
		if idle {
			return
		}
		select {
		case <-deadline.C:
			return
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// enqueueAdvance schedules an async drive of a run. The background paths (DrainFirings / Recover /
// CheckTimeouts→settleTimeout) use it so a slow node never blocks them. Dedup: a run already in
// progress gets a redrive flag (the active driver re-walks once more — no lost wakeup); a run already
// queued is not re-queued. When the pool is not started, drives inline (synchronous, test/manual-only).
//
// enqueueAdvance 安排一个 run 的异步驱动。后台路径用它，故慢节点卡不住它们。去重：已在驱动的 run 置
// redrive 标志（活跃驱动者再走一轮——无丢唤醒）；已入队的 run 不重复入队。池未启动时内联驱动（同步、测试/
// 纯手动）。
func (s *Service) enqueueAdvance(ctx context.Context, runID string) {
	s.advMu.Lock()
	if !s.advStarted {
		s.advMu.Unlock()
		s.drive(ctx, runID) // no pool — drive inline (tests / manual-only deployments)
		return
	}
	if s.advInProgress[runID] {
		s.advPending[runID] = true
		s.advMu.Unlock()
		return
	}
	if s.advQueued[runID] {
		s.advMu.Unlock()
		return
	}
	s.advQueued[runID] = true
	q := s.advQueue
	s.advMu.Unlock()
	s.sendJob(q, advanceJob{ctx, runID}, runID)
}

// sendJob delivers a job to the worker queue, recovering from the "send on closed channel" panic that a
// concurrent StopPool (close(q)) can cause — the send happens after advMu is released (it must not block
// the lock), so it races close. A closed queue means the pool is already shutting down: the run's ctx is
// covered by Shutdown's cancel-all and the run resumes next boot via Recover, so DROPPING the enqueue is
// safe (we just clear its dedup slot). Without this guard a late enqueue racing StopPool would crash the
// whole single-process backend — and Shutdown now bounds its drain waits (build.go) so it CAN reach
// StopPool while a feeder is still mid-send. record-once keeps durability intact regardless (F101).
//
// sendJob 把 job 投到 worker 队列，并从并发 StopPool（close(q)）可能引发的「向已关闭 channel 发送」panic 中
// 恢复——发送在释放 advMu 之后（不能持锁发送以免阻塞），故与 close 竞争。队列已关 = 池正在关停：run 的 ctx 已被
// Shutdown 的 cancel-all 覆盖、下次 boot 经 Recover 续跑，故**丢弃**该入队是安全的（只清其去重槽）。无此保护，
// 一个与 StopPool 竞争的迟到入队会崩掉整个单进程后端——而 Shutdown 现在给排空等待设了上界（build.go），故它
// **能**在某 feeder 仍在 mid-send 时就走到 StopPool。无论如何 record-once 保住持久性（F101）。
func (s *Service) sendJob(q chan advanceJob, job advanceJob, runID string) {
	defer func() {
		if recover() != nil {
			s.advMu.Lock()
			delete(s.advQueued, runID)
			s.advMu.Unlock()
		}
	}()
	q <- job
}

// drive is the single-flight advancer: AT MOST ONE goroutine drives a given run at a time. If another
// goroutine is already driving it, drive records a redrive (the active driver re-walks once more,
// catching whatever state this caller would have advanced) and returns. This collapses concurrent
// triggers for one run into one serial driver — record-once protects DURABILITY against any race, the
// guard prevents the WASTED / duplicate side-effecting activity. Manual paths (StartRun / DecideApproval
// / Replay) call drive inline on the request goroutine and, being the sole driver of a fresh / parked /
// failed run (none of which the pool drives), run synchronously to quiescence — preserving StartRun's
// "returns only after the run reaches terminal/parked" contract. The error returned is the last walk's.
//
// drive 是单飞驱动器：同一 run 同时**至多一个** goroutine 驱动。若另一 goroutine 已在驱动，drive 记一个
// redrive（活跃驱动者再走一轮、接住本调用本会推进的状态）并返回。这把一个 run 的并发触发收敛成一个串行
// 驱动者——record-once 护**持久性**抵御任何 race、guard 防**浪费的/重复的**副作用活动。手动路径
// （StartRun/DecideApproval/Replay）在请求 goroutine 上内联调 drive，作为 fresh/parked/failed run（池都不
// 驱动它们）的唯一驱动者**同步**跑到静止——保住 StartRun「跑到终态/parked 才返回」契约。返回末轮 walk 的错。
func (s *Service) drive(ctx context.Context, runID string) error {
	s.advMu.Lock()
	delete(s.advQueued, runID) // we're handling it now
	if s.advInProgress[runID] {
		s.advPending[runID] = true // another goroutine drives it — ask it to re-walk once
		s.advMu.Unlock()
		return nil
	}
	s.advInProgress[runID] = true
	s.advMu.Unlock()

	var err error
	for {
		err = s.Advance(ctx, runID) // one full walk to quiescence (terminal / parked / interrupted)
		s.advMu.Lock()
		// Redrive only if a signal arrived mid-walk AND ctx is still live. A cancelled run must NOT be
		// re-walked: Advance returns ctx.Err immediately on a dead ctx, so a signal-storm during shutdown
		// would spin this loop hot with no forward progress (an F101 CPU-pin vector). Clear advPending on
		// EVERY exit so a pending redrive can't leak past the in-progress slot's release.
		//
		// 仅当 mid-walk 来了信号 **且** ctx 仍活时才 redrive。已取消的 run 绝不可再走：ctx 死后 Advance 立刻
		// 返 ctx.Err，故关停期的信号风暴会让本循环空转钉 CPU（F101 钉 CPU 的一个向量）。每个出口都清 advPending，
		// 免 pending redrive 泄漏过 in-progress 槽的释放。
		if s.advPending[runID] && ctx.Err() == nil {
			delete(s.advPending, runID)
			s.advMu.Unlock()
			continue // a signal arrived mid-walk — walk once more
		}
		delete(s.advPending, runID)
		delete(s.advInProgress, runID)
		s.advMu.Unlock()
		return err
	}
}
