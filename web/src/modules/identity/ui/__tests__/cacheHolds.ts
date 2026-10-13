import type { QueryClient } from '@tanstack/svelte-query'

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
		.map((query) => query.state.data)
	return JSON.stringify([mutations, queries]).includes(secret)
}
