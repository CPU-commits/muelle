package usecases

import (
	"sync"
	"time"
)

// NotSyncedYet es el error de GET /proxy desde que arranca muelle hasta el
// primer POST /load (contrato §3.3).
const NotSyncedYet = "muelle todavía no le mandó la configuración a Caddy"

// ProxyStatus es el status de GET /proxy (contrato §3.3).
type ProxyStatus string

const (
	ProxySynced     ProxyStatus = "synced"
	ProxyOutOfSync  ProxyStatus = "out_of_sync"
	ProxyRecreating ProxyStatus = "recreating"
	ProxyDisabled   ProxyStatus = "disabled"
)

// ProxyState es el estado del proxy en memoria: el último error del
// contenedor y el de POST /load, el último Load exitoso y las recreaciones
// pendientes. Lo escriben el Syncer y (desde T3) el ciclo de vida de
// muelle-caddy; lo lee GET /proxy. Se puede usar desde varias goroutines.
type ProxyState struct {
	mu           sync.Mutex
	containerErr string
	loadErr      string
	syncedAt     *time.Time
	// gen es la última recreación agendada y endedGen la última terminada:
	// hay una pendiente mientras gen > endedGen (D15 del plan de la Fase 3).
	gen, endedGen uint64
}

func NewProxyState() *ProxyState { return &ProxyState{} }

// Loaded registra un POST /load exitoso: limpia su error.
func (s *ProxyState) Loaded(at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadErr, s.syncedAt = "", &at
}

// LoadFailed registra un POST /load que falló (o que no se pudo armar).
// msg ya viene sin el token.
func (s *ProxyState) LoadFailed(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.loadErr = msg
}

// ContainerOK limpia el error del contenedor.
func (s *ProxyState) ContainerOK() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.containerErr = ""
}

// ContainerFailed registra que muelle-caddy no se pudo crear o recrear.
func (s *ProxyState) ContainerFailed(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.containerErr = msg
}

// BeginRecreate agenda una recreación y devuelve su generación: GET /proxy
// dice recreating hasta que termina la última (EndRecreate).
func (s *ProxyState) BeginRecreate() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gen++
	return s.gen
}

// Generation es la última recreación agendada. El Recreator la toma antes de
// recrear y termina con EndRecreate(gen): lo que se agende mientras recrea
// sigue pendiente.
func (s *ProxyState) Generation() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen
}

// EndRecreate marca terminadas las recreaciones hasta gen. Una que se agendó
// después sigue pendiente, así un cambio que llegó durante la recreación no
// se pierde.
func (s *ProxyState) EndRecreate(gen uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.endedGen = max(s.endedGen, gen)
}

// Recreating dice si hay una recreación agendada o en curso.
func (s *ProxyState) Recreating() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen > s.endedGen
}

// Synced dice si Caddy tiene la última configuración: sin recreación
// pendiente, sin errores y con al menos un Load exitoso.
func (s *ProxyState) Synced() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen <= s.endedGen && s.containerErr == "" && s.loadErr == "" && s.syncedAt != nil
}

// Snapshot es el estado para GET /proxy (contrato §3.3), en este orden:
// disabled si !enabled; recreating si hay una recreación pendiente; si no,
// out_of_sync con el error del contenedor (gana), el de /load o
// NotSyncedYet; si no, synced. errMsg es "" salvo con out_of_sync. syncedAt
// es una copia del último Load exitoso (nil si no hubo).
func (s *ProxyState) Snapshot(enabled bool) (status ProxyStatus, errMsg string, syncedAt *time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.syncedAt != nil {
		t := *s.syncedAt
		syncedAt = &t
	}
	switch {
	case !enabled:
		return ProxyDisabled, "", syncedAt
	case s.gen > s.endedGen:
		return ProxyRecreating, "", syncedAt
	case s.containerErr != "":
		return ProxyOutOfSync, s.containerErr, syncedAt
	case s.loadErr != "":
		return ProxyOutOfSync, s.loadErr, syncedAt
	case syncedAt == nil:
		return ProxyOutOfSync, NotSyncedYet, nil
	}
	return ProxySynced, "", syncedAt
}
