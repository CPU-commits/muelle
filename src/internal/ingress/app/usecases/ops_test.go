package usecases

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Dos Do a la vez nunca corren juntos, y Do devuelve el error de fn.
func TestOpsSerializes(t *testing.T) {
	var o Ops
	var inside, maxInside atomic.Int32
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			o.Do(context.Background(), time.Second, func(opCtx, dbCtx context.Context) error {
				n := inside.Add(1)
				for {
					m := maxInside.Load()
					if n <= m || maxInside.CompareAndSwap(m, n) {
						break
					}
				}
				time.Sleep(2 * time.Millisecond)
				inside.Add(-1)
				return nil
			})
		})
	}
	wg.Wait()
	if maxInside.Load() != 1 {
		t.Fatalf("corrieron %d a la vez", maxInside.Load())
	}
	boom := errors.New("boom")
	if err := o.Do(context.Background(), time.Second, func(context.Context, context.Context) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("Do: %v", err)
	}
}

// El plazo corre desde antes de esperar el lock (D6): el que esperó llega
// con menos tiempo.
func TestOpsDeadlineIncludesWait(t *testing.T) {
	var o Ops
	locked := make(chan time.Time)
	go o.Do(context.Background(), time.Second, func(context.Context, context.Context) error {
		locked <- time.Now()
		time.Sleep(40 * time.Millisecond)
		return nil
	})
	lockedAt := <-locked
	start := time.Now()
	var remaining time.Duration
	var deadline time.Time
	o.Do(context.Background(), 50*time.Millisecond, func(opCtx, dbCtx context.Context) error {
		deadline, _ = opCtx.Deadline()
		remaining = time.Until(deadline)
		return nil
	})
	if deadline.After(start.Add(52 * time.Millisecond)) {
		t.Fatalf("el plazo %s no cuenta desde antes del lock (%s)", deadline, start)
	}
	if limit := 10*time.Millisecond + start.Sub(lockedAt) + 2*time.Millisecond; remaining > limit {
		t.Fatalf("le quedaban %s; esperaba a lo más %s", remaining, limit)
	}
}

// Si ctx vence antes que limit, opCtx usa el plazo de ctx; dbCtx no vence.
func TestOpsShorterParent(t *testing.T) {
	var o Ops
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	parent, _ := ctx.Deadline()
	o.Do(ctx, time.Minute, func(opCtx, dbCtx context.Context) error {
		if d, ok := opCtx.Deadline(); !ok || !d.Equal(parent) {
			t.Errorf("opCtx vence %v (%v), esperaba %v", d, ok, parent)
		}
		<-opCtx.Done()
		if _, ok := dbCtx.Deadline(); ok || dbCtx.Err() != nil {
			t.Errorf("dbCtx: %v", dbCtx.Err())
		}
		return nil
	})
}

// Un ctx ya cancelado (el cliente se fue) corre fn igual (D6).
func TestOpsCanceledParent(t *testing.T) {
	var o Ops
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ran := false
	o.Do(ctx, time.Minute, func(opCtx, dbCtx context.Context) error {
		ran = true
		if opCtx.Err() != nil || dbCtx.Err() != nil {
			t.Errorf("opCtx %v, dbCtx %v", opCtx.Err(), dbCtx.Err())
		}
		return nil
	})
	if !ran {
		t.Fatal("fn no corrió")
	}
}
