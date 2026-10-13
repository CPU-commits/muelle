import type { QueryClient } from '@tanstack/svelte-query'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import ViewModelHarness from './ViewModelHarness.svelte'

/** Las mismas opciones que la app, sin reintentos. */
export function testQueryClient(): QueryClient {
	return createQueryClient({ retry: false })
}

/** Crea el viewModel dentro del harness y lo devuelve. */
export async function mountViewModel<T>(
	create: () => T,
	client: QueryClient = testQueryClient(),
): Promise<T> {
	let vm: T | undefined
	await render(ViewModelHarness<T>, {
		client,
		create,
		onReady: (instance: T) => {
			vm = instance
		},
	})
	if (vm === undefined) throw new Error('el harness no expuso el viewModel')
	return vm
}

/** Espera a que una condición sobre el estado reactivo se cumpla. */
export async function until(condition: () => boolean, timeoutMs = 2000): Promise<void> {
	const start = Date.now()
	while (!condition()) {
		if (Date.now() - start > timeoutMs) throw new Error('la condición no se cumplió a tiempo')
		await new Promise((resolve) => setTimeout(resolve, 10))
	}
}
