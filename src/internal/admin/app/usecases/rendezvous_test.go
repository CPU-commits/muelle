package usecases

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/CPU-commits/muelle/src/internal/admin/domain/repository"
	"github.com/CPU-commits/muelle/src/internal/admin/domain/service"
	adminrepo "github.com/CPU-commits/muelle/src/internal/admin/infra/out/repository/sqlite"
	"github.com/CPU-commits/muelle/src/shared/infra/out/crypto"
)

// rendezvousRepo hace que cada Get espere (hasta 300 ms) a que el otro
// arranque también haya leído: fuerza la carrera Get/Get/Set/Set si el caso
// de uso no es atómico.
type rendezvousRepo struct {
	repository.SettingsRepository
	mu   sync.Mutex
	n    int
	both chan struct{}
}

func (r *rendezvousRepo) Get(ctx context.Context, key string) ([]byte, bool, error) {
	v, enc, err := r.SettingsRepository.Get(ctx, key)
	r.mu.Lock()
	r.n++
	if r.n == 2 {
		close(r.both)
	}
	r.mu.Unlock()
	select {
	case <-r.both:
	case <-time.After(300 * time.Millisecond):
	}
	return v, enc, err
}

// Determinista: si la verificación no fuera atómica, los dos arranques
// leerían "no existe" a la vez y los dos pasarían.
func TestVerifyMasterKeyRaceIsDeterministic(t *testing.T) {
	e := newEnv(t)
	repo := &rendezvousRepo{SettingsRepository: adminrepo.NewSettingsRepository(e.db), both: make(chan struct{})}
	keyA, _ := crypto.New(bytes.Repeat([]byte{7}, crypto.KeySize))
	keyB, _ := crypto.New(bytes.Repeat([]byte{8}, crypto.KeySize))
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, k := range []*crypto.Cipher{keyA, keyB} {
		wg.Go(func() { errs[i] = NewVerifyMasterKey(repo, k, e.db).Execute(context.Background()) })
	}
	wg.Wait()
	passed := 0
	for _, err := range errs {
		if err == nil {
			passed++
		} else if !errors.Is(err, service.ErrWrongMasterKey) {
			t.Fatalf("err inesperado: %v", err)
		}
	}
	if passed != 1 {
		t.Fatalf("pasaron %d claves: %v", passed, errs)
	}
}
