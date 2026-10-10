// Con `pnpm dev:mock` los handlers corren en la página: al recargar, la memoria se
// pierde. Cada db puede guardarse en sessionStorage (por pestaña) para que la sesión y
// lo creado sobrevivan a un F5. En node (tests) queda en memoria: Node 26 también tiene
// un sessionStorage, y compartirlo entre archivos de test haría que uno herede el estado
// del anterior.
const PREFIX = 'muelle-mock:'

function storage(): Storage | null {
	if (typeof window === 'undefined' || typeof sessionStorage === 'undefined') return null
	return sessionStorage
}

export function loadPersisted<T>(key: string): T | null {
	try {
		const raw = storage()?.getItem(PREFIX + key)
		return raw ? (JSON.parse(raw) as T) : null
	} catch {
		return null
	}
}

export function savePersisted(key: string, value: unknown): void {
	try {
		storage()?.setItem(PREFIX + key, JSON.stringify(value))
	} catch {
		// Sin persistencia, los mocks siguen en memoria hasta recargar.
	}
}
