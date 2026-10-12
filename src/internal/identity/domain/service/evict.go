package service

import (
	"slices"
	"time"
)

// evictSoonest borra de m las entradas que vencen antes (según end) hasta
// dejar target. Nunca elige al azar: lo que más tiempo le queda (un bloqueo
// largo, una ventana recién empezada) es lo último que se desaloja, así una
// avalancha de claves nuevas no borra justo el bloqueo de un atacante.
func evictSoonest[K comparable, V any](m map[K]V, target int, end func(V) time.Time) {
	n := len(m) - target
	if n <= 0 {
		return
	}
	type item struct {
		key K
		end time.Time
	}
	items := make([]item, 0, len(m))
	for k, v := range m {
		items = append(items, item{k, end(v)})
	}
	slices.SortFunc(items, func(a, b item) int { return a.end.Compare(b.end) })
	for _, it := range items[:n] {
		delete(m, it.key)
	}
}
