import { describe, expect, it } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import MaskedText from '../MaskedText.svelte'

describe('MaskedText', () => {
	it('marca cada *** como secreto oculto, con el texto para el lector de pantalla', async () => {
		const screen = await render(MaskedText, { text: 'DATABASE_URL=*** y TOKEN=***' })
		const element = screen.container

		expect(element.textContent).toBe(
			`DATABASE_URL=***${m.deploy_masked()} y TOKEN=***${m.deploy_masked()}`,
		)
		const marks = element.querySelectorAll(`[title="${m.deploy_masked()}"]`)
		expect(marks).toHaveLength(2)
		// Lo visible (`***`) no se lee: se lee "secreto oculto".
		for (const mark of marks) {
			expect(mark.querySelector('[aria-hidden="true"]')?.textContent).toBe('***')
		}
	})

	it('una línea sin *** queda igual, sin marcas', async () => {
		const screen = await render(MaskedText, { text: 'Step 3/7 : RUN pnpm install' })

		expect(screen.container.textContent).toBe('Step 3/7 : RUN pnpm install')
		expect(screen.container.querySelector('[title]')).toBeNull()
	})

	it('el texto del log nunca se interpreta como HTML', async () => {
		const hostile =
			'<img src=x onerror="window.__pwned = true"><b>negrita</b> *** <script>x</script>'
		const screen = await render(MaskedText, { text: hostile })

		expect(screen.container.querySelector('img, b, script')).toBeNull()
		expect(screen.container.textContent).toBe(
			`<img src=x onerror="window.__pwned = true"><b>negrita</b> ***${m.deploy_masked()} <script>x</script>`,
		)
		expect((window as { __pwned?: boolean }).__pwned).toBeUndefined()
	})
})
