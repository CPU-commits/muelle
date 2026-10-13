import { QueryClient } from '@tanstack/svelte-query'
import { render } from 'vitest-browser-svelte'
import ViewModelHarness from './ViewModelHarness.svelte'

/** Un QueryClient sin reintentos, para que un error llegue enseguida. */
export function testClient(): QueryClient {
	return new QueryClient({
		defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
	})
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
