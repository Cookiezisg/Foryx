package handler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

type fakeClient struct {
	mu       sync.Mutex
	crashed  bool
	shutdown bool
}

func (f *fakeClient) Init(context.Context, map[string]any) error        { return nil }
func (f *fakeClient) Call(context.Context, string, map[string]any) (any, error) { return nil, nil }
func (f *fakeClient) StreamCall(context.Context, string, map[string]any, func(any)) (any, error) {
	return nil, nil
}
func (f *fakeClient) Shutdown(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shutdown = true
	return nil
}
func (f *fakeClient) Crashed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.crashed
}
func (f *fakeClient) markCrashed() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.crashed = true
}
func (f *fakeClient) isShutdown() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.shutdown
}

type killCounter struct{ count atomic.Int32 }

func (k *killCounter) kill() error { k.count.Add(1); return nil }

func mkInstance(t *testing.T, owner Owner, handlerName string) (*Instance, *fakeClient, *killCounter) {
	t.Helper()
	fc := &fakeClient{}
	kc := &killCounter{}
	return &Instance{
		ID:        NewInstanceID(),
		HandlerID: "hd_test_" + handlerName,
		Owner:     owner,
		Client:    fc,
		Kill:      kc.kill,
	}, fc, kc
}

func TestAcquire_FirstCallSpawns(t *testing.T) {
	r := newInstanceRegistry()
	owner := Owner{Kind: "workflow", ID: "run_001"}
	spawnCount := atomic.Int32{}

	inst, err := r.Acquire(context.Background(), owner, "pg", func(ctx context.Context) (*Instance, error) {
		spawnCount.Add(1)
		ins, _, _ := mkInstance(t, owner, "pg")
		return ins, nil
	})
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	if spawnCount.Load() != 1 {
		t.Errorf("spawnCount = %d, want 1", spawnCount.Load())
	}
	if inst.Owner != owner {
		t.Errorf("Owner mismatch: %v", inst.Owner)
	}
	if r.CountForOwner(owner) != 1 {
		t.Errorf("CountForOwner = %d, want 1", r.CountForOwner(owner))
	}
}

func TestAcquire_SecondCallReuses(t *testing.T) {
	r := newInstanceRegistry()
	owner := Owner{Kind: "workflow", ID: "run_001"}
	spawnCount := atomic.Int32{}
	spawn := func(ctx context.Context) (*Instance, error) {
		spawnCount.Add(1)
		ins, _, _ := mkInstance(t, owner, "pg")
		return ins, nil
	}

	inst1, _ := r.Acquire(context.Background(), owner, "pg", spawn)
	inst2, _ := r.Acquire(context.Background(), owner, "pg", spawn)
	if inst1 != inst2 {
		t.Errorf("Acquire reuse failed: pointers differ")
	}
	if spawnCount.Load() != 1 {
		t.Errorf("spawnCount = %d, want 1 (second Acquire should reuse)", spawnCount.Load())
	}
}

func TestAcquire_CrashedRespawns(t *testing.T) {
	r := newInstanceRegistry()
	owner := Owner{Kind: "workflow", ID: "run_001"}
	spawnCount := atomic.Int32{}
	var lastFC *fakeClient
	spawn := func(ctx context.Context) (*Instance, error) {
		spawnCount.Add(1)
		ins, fc, _ := mkInstance(t, owner, "pg")
		lastFC = fc
		return ins, nil
	}

	_, _ = r.Acquire(context.Background(), owner, "pg", spawn)
	lastFC.markCrashed() // simulate crash

	_, _ = r.Acquire(context.Background(), owner, "pg", spawn)
	if spawnCount.Load() != 2 {
		t.Errorf("spawnCount after crashed Acquire = %d, want 2", spawnCount.Load())
	}
}

func TestAcquire_CrossOwnerIsolation(t *testing.T) {
	r := newInstanceRegistry()
	o1 := Owner{Kind: "workflow", ID: "run_A"}
	o2 := Owner{Kind: "workflow", ID: "run_B"}
	spawn := func(owner Owner) SpawnFn {
		return func(ctx context.Context) (*Instance, error) {
			ins, _, _ := mkInstance(t, owner, "pg")
			return ins, nil
		}
	}

	inst1, _ := r.Acquire(context.Background(), o1, "pg", spawn(o1))
	inst2, _ := r.Acquire(context.Background(), o2, "pg", spawn(o2))
	if inst1 == inst2 {
		t.Error("cross-owner Acquire returned same instance")
	}
	if inst1.Owner.ID != "run_A" || inst2.Owner.ID != "run_B" {
		t.Errorf("owner IDs swapped: %v vs %v", inst1.Owner, inst2.Owner)
	}
}

func TestDestroyOwner_ShutsAndKills(t *testing.T) {
	r := newInstanceRegistry()
	owner := Owner{Kind: "test", ID: "t_001"}
	ins, fc, kc := mkInstance(t, owner, "pg")
	_, _ = r.Acquire(context.Background(), owner, "pg", func(ctx context.Context) (*Instance, error) {
		return ins, nil
	})

	r.DestroyOwner(context.Background(), owner)

	if !fc.isShutdown() {
		t.Error("Client.Shutdown not called on DestroyOwner")
	}
	if kc.count.Load() != 1 {
		t.Errorf("Kill calls = %d, want 1", kc.count.Load())
	}
	if r.CountForOwner(owner) != 0 {
		t.Errorf("CountForOwner after Destroy = %d, want 0", r.CountForOwner(owner))
	}
}

func TestDestroyOwner_OtherOwnersUntouched(t *testing.T) {
	r := newInstanceRegistry()
	o1 := Owner{Kind: "workflow", ID: "A"}
	o2 := Owner{Kind: "workflow", ID: "B"}

	ins1, _, kc1 := mkInstance(t, o1, "pg")
	ins2, _, kc2 := mkInstance(t, o2, "pg")
	_, _ = r.Acquire(context.Background(), o1, "pg", func(ctx context.Context) (*Instance, error) { return ins1, nil })
	_, _ = r.Acquire(context.Background(), o2, "pg", func(ctx context.Context) (*Instance, error) { return ins2, nil })

	r.DestroyOwner(context.Background(), o1)

	if kc1.count.Load() != 1 {
		t.Errorf("o1 Kill = %d, want 1", kc1.count.Load())
	}
	if kc2.count.Load() != 0 {
		t.Errorf("o2 Kill = %d, want 0 (untouched)", kc2.count.Load())
	}
	if r.CountForOwner(o2) != 1 {
		t.Errorf("o2 count = %d, want 1", r.CountForOwner(o2))
	}
}

func TestDestroyEverything(t *testing.T) {
	r := newInstanceRegistry()
	owners := []Owner{
		{Kind: "workflow", ID: "A"},
		{Kind: "test", ID: "T"},
		{Kind: "session", ID: "S"},
	}
	kcs := []*killCounter{}
	for _, o := range owners {
		ins, _, kc := mkInstance(t, o, "pg")
		kcs = append(kcs, kc)
		_, _ = r.Acquire(context.Background(), o, "pg", func(ctx context.Context) (*Instance, error) { return ins, nil })
	}

	r.DestroyEverything(context.Background())

	for i, kc := range kcs {
		if kc.count.Load() != 1 {
			t.Errorf("owner %d Kill = %d, want 1", i, kc.count.Load())
		}
	}
	for _, o := range owners {
		if r.CountForOwner(o) != 0 {
			t.Errorf("owner %v count = %d, want 0", o, r.CountForOwner(o))
		}
	}
}

func TestSnapshot_ReflectsLiveInstances(t *testing.T) {
	r := newInstanceRegistry()
	owner := Owner{Kind: "workflow", ID: "X"}
	ins1, _, _ := mkInstance(t, owner, "pg")
	ins2, _, _ := mkInstance(t, owner, "redis")
	_, _ = r.Acquire(context.Background(), owner, "pg", func(ctx context.Context) (*Instance, error) { return ins1, nil })
	_, _ = r.Acquire(context.Background(), owner, "redis", func(ctx context.Context) (*Instance, error) { return ins2, nil })

	snap := r.Snapshot()
	inner := snap[owner]
	if inner["pg"] != ins1.ID || inner["redis"] != ins2.ID {
		t.Errorf("snapshot mismatch: %+v", inner)
	}
}

func TestNewInstanceID_Prefix(t *testing.T) {
	id := NewInstanceID()
	if id == "" || len(id) < 4 || id[:4] != "hdi_" {
		t.Errorf("NewInstanceID = %q, want hdi_<hex>", id)
	}
}

func TestAcquire_SpawnError(t *testing.T) {
	r := newInstanceRegistry()
	owner := Owner{Kind: "test", ID: "fail"}
	want := errors.New("spawn boom")
	_, err := r.Acquire(context.Background(), owner, "pg", func(ctx context.Context) (*Instance, error) {
		return nil, want
	})
	if !errors.Is(err, want) {
		t.Errorf("Acquire propagation: %v", err)
	}
	if r.CountForOwner(owner) != 0 {
		t.Errorf("failed Acquire should not register; count = %d", r.CountForOwner(owner))
	}
}
