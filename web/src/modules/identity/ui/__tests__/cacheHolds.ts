import type { QueryClient } from '@tanstack/svelte-query'

/**
 * Un `Error` como objeto plano: `JSON.stringify` de un Error es `{}`, y un secreto en su
 * mensaje, su stack, su `cause` o cualquier propiedad propia pasaría sin verse.
 */
function errorsAsObjects(_key: string, value: unknown): unknown {
	if (!(value instanceof Error)) return value
	const plain: Record<string, unknown> = { name: value.name }
	for (const key of Object.getOwnPropertyNames(value)) {
		plain[key] = (value as unknown as Record<string, unknown>)[key]
	}
	return plain
}

/**
 * Si `secret` quedó en algún lado del cache de TanStack: en los datos de una query o en el
 * estado (variables, resultado, error) de una mutación. Espera un momento antes de mirar:
 * una mutación con `gcTime: 0` se borra en el tick siguiente a quedarse sin observers.
 */
export async function cacheHolds(client: QueryClient, secret: string): Promise<boolean> {
	await new Promise((resolve) => setTimeout(resolve, 20))
	const mutations = client
		.getMutationCache()
		.getAll()
		.map((mutation) => mutation.state)
	const queries = client
		.getQueryCache()
		.getAll()
		.map((query) => ({ data: query.state.data, error: query.state.error }))
	return JSON.stringify([mutations, queries], errorsAsObjects).includes(secret)
}
