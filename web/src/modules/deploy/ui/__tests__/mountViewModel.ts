import type { QueryClient } from '@tanstack/svelte-query'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import ViewModelHarness from './ViewModelHarness.svelte'

/** El QueryClient de la app sin reintentos, para que un error llegue enseguida. */
export function testClient(): QueryClient {
	return createQueryClient({ retry: false })
}

/** Monta el viewModel que devuelve `create` y lo devuelve, con su QueryClient. */
export async function mountViewModel<T>(
	create: () => T,
	client: QueryClient = testClient(),
): Promise<{ vm: T; client: QueryClient; unmount: () => void }> {
	let vm: T | undefined
	const screen = await render(ViewModelHarness, {
		client,
		create,
		onReady: (instance: unknown) => {
			vm = instance as T
		},
	})
	if (vm === undefined) throw new Error('el harness no expuso el viewModel')
	return { vm, client, unmount: () => screen.unmount() }
}
