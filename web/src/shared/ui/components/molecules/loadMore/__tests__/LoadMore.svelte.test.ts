import { describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import LoadMore from '../LoadMore.svelte'

describe('LoadMore', () => {
	it('con más páginas, "Cargar más" pide la siguiente', async () => {
		const onLoadMore = vi.fn()
		const screen = await render(LoadMore, { hasMore: true, onLoadMore })

		await screen.getByRole('button', { name: m.common_load_more() }).click()

		expect(onLoadMore).toHaveBeenCalledOnce()
	})

	it('mientras carga no vuelve a pedir', async () => {
		const onLoadMore = vi.fn()
		const screen = await render(LoadMore, { hasMore: true, loading: true, onLoadMore })

		;(screen.getByRole('button').element() as HTMLButtonElement).click()

		expect(onLoadMore).not.toHaveBeenCalled()
	})

	it('sin más, no hay botón y dice el texto de fin si se pasa', async () => {
		const screen = await render(LoadMore, {
			hasMore: false,
			onLoadMore: vi.fn(),
			doneLabel: m.common_no_more(),
		})

		await expect.element(screen.getByRole('button')).not.toBeInTheDocument()
		await expect.element(screen.getByText(m.common_no_more())).toBeVisible()
	})
})
