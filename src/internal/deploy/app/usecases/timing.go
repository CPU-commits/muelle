package usecases

import "time"

// Timing son los plazos del worker de deploy (decisión 13 del plan); los
// tests los acortan.
type Timing struct {
	// Poll es cada cuánto se mira Swarm al esperar la convergencia (§7 paso 6).
	Poll time.Duration
	// Settle es el margen de un servicio que ya existía y no cambió: converge
	// pasado Settle desde que docker stack deploy volvió (decisión 6).
	Settle time.Duration
	// ResolveTimeout es el plazo de cada imagen al resolverla (paso 2).
	ResolveTimeout time.Duration
	// NetworkTimeout es el plazo de EnsureStageNetwork (paso 4).
	NetworkTimeout time.Duration
	// StackTimeout es el plazo de docker stack deploy (paso 5).
	StackTimeout time.Duration
	// RoutesTimeout es el plazo de SyncRoutes (paso 7).
	RoutesTimeout time.Duration
	// RetryEvery es cada cuánto la reconciliación vuelve a mirar Swarm con
	// Docker caído, y cuánto espera un worker después de un error interno
	// (contrato §1.2).
	RetryEvery time.Duration
}

// DefaultTiming son los plazos de §7 y del contrato §1.2.
func DefaultTiming() Timing {
	return Timing{
		Poll:           2 * time.Second,
		Settle:         5 * time.Second,
		ResolveTimeout: 60 * time.Second,
		NetworkTimeout: 30 * time.Second,
		StackTimeout:   2 * time.Minute,
		RoutesTimeout:  30 * time.Second,
		RetryEvery:     15 * time.Second,
	}
}
