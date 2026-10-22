import { describe, expect, it } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import type { EnvKey } from '../../../../domain'
import EnvValueText from '../EnvValueText.svelte'

const updatedAt = new Date('2026-10-20T12:00:00Z')
const key = (raw: string) => raw as EnvKey

describe('EnvValueText', () => {
	it('una secreta muestra el candado y "valor secreto", sin ningún valor', async () => {
		const screen = await render(EnvValueText, {
			envVar: { key: key('API_TOKEN'), secret: true, value: null, updatedAt },
		})

		await expect.element(screen.getByText(m.secrets_secret_masked())).toBeInTheDocument()
		const text = screen.container.textContent ?? ''
		expect(text).toContain('••••')
		expect(screen.container.querySelector('[title]')).toBeNull()
	})

	it('una que no es secreta muestra su valor, entero en el title', async () => {
		const screen = await render(EnvValueText, {
			envVar: { key: key('PORT'), secret: false, value: '3000', updatedAt },
		})

		await expect.element(screen.getByText('3000')).toHaveAttribute('title', '3000')
		expect(screen.container.textContent).not.toContain(m.secrets_secret_masked())
	})

	it('una vacía lo dice', async () => {
		const screen = await render(EnvValueText, {
			envVar: { key: key('EMPTY'), secret: false, value: '', updatedAt },
		})

		await expect.element(screen.getByText(m.secrets_value_empty())).toBeInTheDocument()
	})
})
