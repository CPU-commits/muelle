package tools

// CallsInFlight es cuántos principales tienen llamadas en curso: el mapa
// borra la entrada de uno que llega a 0.
func CallsInFlight(k *Kit) int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.calls)
}
