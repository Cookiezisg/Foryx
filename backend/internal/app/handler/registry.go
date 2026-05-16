package handler

import (
	"context"
	"sync"

	handlerinfra "github.com/sunweilin/forgify/backend/internal/infra/handler"
	idgenpkg "github.com/sunweilin/forgify/backend/internal/pkg/idgen"
)

// Owner identifies a caller-context scope for instance lifetime.
//
// Owner 标识用于 instance 生命周期的 caller-context scope。
type Owner struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// Instance is one live HandlerInstance subprocess plus its RPC client.
//
// Instance 是一个活的 HandlerInstance 子进程加其 RPC 客户端。
type Instance struct {
	ID        string
	HandlerID string
	Owner     Owner
	Client    handlerinfra.Client
	Kill      func() error
}

type instanceRegistry struct {
	mu        sync.Mutex
	instances map[Owner]map[string]*Instance
}

func newInstanceRegistry() *instanceRegistry {
	return &instanceRegistry{
		instances: make(map[Owner]map[string]*Instance),
	}
}

// SpawnFn builds a fresh Instance when Acquire has no live one.
//
// SpawnFn 在 Acquire 找不到活 instance 时构造新 Instance。
type SpawnFn func(ctx context.Context) (*Instance, error)

// Acquire returns the live instance for (owner, handlerName), spawning if missing or crashed.
//
// Acquire 返 (owner, handlerName) 的活 instance；不存在或 crashed 时调 spawnFn。
func (r *instanceRegistry) Acquire(ctx context.Context, owner Owner, handlerName string, spawnFn SpawnFn) (*Instance, error) {
	r.mu.Lock()
	if om, ok := r.instances[owner]; ok {
		if inst, ok := om[handlerName]; ok && !inst.Client.Crashed() {
			r.mu.Unlock()
			return inst, nil
		}
		if inst, ok := om[handlerName]; ok {
			_ = inst.Client.Shutdown(ctx)
			_ = inst.Kill()
			delete(om, handlerName)
		}
	}
	r.mu.Unlock()

	inst, err := spawnFn(ctx)
	if err != nil {
		return nil, err
	}

	// Race: another goroutine may have spawned concurrently; prefer the registered one.
	// 竞态：并发 goroutine 可能已注册一份，优先用已注册的。
	r.mu.Lock()
	defer r.mu.Unlock()
	om, ok := r.instances[owner]
	if !ok {
		om = make(map[string]*Instance)
		r.instances[owner] = om
	}
	if existing, ok := om[handlerName]; ok && !existing.Client.Crashed() {
		go func() {
			_ = inst.Client.Shutdown(context.Background())
			_ = inst.Kill()
		}()
		return existing, nil
	}
	om[handlerName] = inst
	return inst, nil
}

// DestroyOwner destroys every instance scoped to the given owner.
//
// DestroyOwner 销毁某 owner 下的所有 instance。
func (r *instanceRegistry) DestroyOwner(ctx context.Context, owner Owner) {
	r.mu.Lock()
	om := r.instances[owner]
	delete(r.instances, owner)
	r.mu.Unlock()

	for _, inst := range om {
		_ = inst.Client.Shutdown(ctx)
		_ = inst.Kill()
	}
}

// DestroyEverything tears down every live instance across all owners.
//
// DestroyEverything 销毁所有 owner 的全部活 instance。
func (r *instanceRegistry) DestroyEverything(ctx context.Context) {
	r.mu.Lock()
	owners := make([]Owner, 0, len(r.instances))
	for o := range r.instances {
		owners = append(owners, o)
	}
	r.mu.Unlock()

	for _, o := range owners {
		r.DestroyOwner(ctx, o)
	}
}

// Snapshot returns a (owner → handlerName → InstanceID) copy for observability.
//
// Snapshot 返 (owner → handlerName → InstanceID) 的拷贝供观测使用。
func (r *instanceRegistry) Snapshot() map[Owner]map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[Owner]map[string]string, len(r.instances))
	for o, om := range r.instances {
		inner := make(map[string]string, len(om))
		for name, inst := range om {
			inner[name] = inst.ID
		}
		out[o] = inner
	}
	return out
}

// CountForOwner returns the live instance count for one owner.
//
// CountForOwner 返某 owner 的活 instance 数。
func (r *instanceRegistry) CountForOwner(owner Owner) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.instances[owner])
}

// NewInstanceID mints a fresh Instance ID with the `hdi_` prefix.
//
// NewInstanceID 用 `hdi_` 前缀生成新 Instance ID。
func NewInstanceID() string {
	return idgenpkg.New("hdi")
}
