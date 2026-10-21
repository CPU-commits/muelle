import { describe, expect, it } from 'vitest'
import { render } from 'vitest-browser-svelte'
import LoadingState from '../LoadingState.svelte'

describe('LoadingState', () => {
	it('se anuncia una sola vez: un status con el texto y el spinner decorativo', async () => {
		const screen = await render(LoadingState, { label: 'Cargando proyectos…' })

		const statuses = screen.container.querySelectorAll('[role="status"]')
		expect(statuses).toHaveLength(1)
		expect(statuses[0]?.textContent?.trim()).toBe('Cargando proyectos…')
		expect(screen.container.querySelectorAll('[aria-label]')).toHaveLength(0)
	})
})
