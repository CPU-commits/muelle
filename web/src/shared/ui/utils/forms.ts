// TanStack Form guarda los errores de un campo como un array heterogeneo
// (strings de los validators, objetos de un schema). La UI muestra el primero.
export function firstError(errors: readonly unknown[]): string | null {
	for (const error of errors) {
		if (typeof error === 'string' && error) return error
		if (error && typeof error === 'object' && 'message' in error) {
			const message = (error as { message?: unknown }).message
			if (typeof message === 'string') return message
		}
	}
	return null
}
