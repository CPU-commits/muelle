import { describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import QueryProbe from './QueryProbe.svelte'

describe('las queries del panel (createQueryClient)', () => {
	it('un viewModel que lee `data` primero igual ve el error cuando lo lee después', async () => {
		const load = vi.fn(() => Promise.reject(new Error('backend caído')))
		const screen = await render(QueryProbe, { load })
		await expect.poll(() => load.mock.calls.length).toBe(1)
		// El pedido ya falló, y lo único que se leyó fue `data`.
		await new Promise((done) => setTimeout(done, 20))
		expect(screen.getByTestId('data').element().textContent).toBe('sin datos')

		await screen.getByRole('button', { name: 'ver error' }).click()

		await expect.element(screen.getByTestId('error')).toHaveTextContent('backend caído')
	})
})
