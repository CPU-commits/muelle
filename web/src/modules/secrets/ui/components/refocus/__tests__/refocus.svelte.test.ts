import { describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import RefocusHarness from './RefocusHarness.svelte'

const focusedText = () => document.activeElement?.textContent?.trim()
const button = (name: string) =>
	[...document.querySelectorAll('button')].find((element) => element.textContent === name)

describe('refocusOnClose', () => {
	it('al montar no mueve el foco', async () => {
		await render(RefocusHarness, { closed: 0 })
		await new Promise(requestAnimationFrame)

		expect(document.activeElement).toBe(document.body)
	})

	it('si el panel se cerró con el foco adentro, el foco vuelve al botón que lo abre', async () => {
		const screen = await render(RefocusHarness, { closed: 0, panelOpen: true })
		button('Cancelar')?.focus()
		expect(focusedText()).toBe('Cancelar')

		// El panel se desmonta con el foco adentro: el foco cae en <body>.
		await screen.rerender({ closed: 1, panelOpen: false })
		await expect.poll(focusedText).toBe('Importar .env')
	})

	it('también desde el respaldo del Modal (el h1 de <main>), cuando su botón ya no está', async () => {
		const screen = await render(RefocusHarness, { closed: 0 })
		document.querySelector<HTMLElement>('h1')?.focus()

		await screen.rerender({ closed: 1 })
		await expect.poll(focusedText).toBe('Importar .env')
	})

	it('si el foco ya está en otra cosa, no se lo saca', async () => {
		const screen = await render(RefocusHarness, { closed: 0, panelOpen: true })
		button('Otra cosa')?.focus()

		await screen.rerender({ closed: 1, panelOpen: false })
		await new Promise(requestAnimationFrame)
		await new Promise(requestAnimationFrame)
		expect(focusedText()).toBe('Otra cosa')
	})
})

describe('focusOnRequest', () => {
	it('sin pedido no hace nada; con uno, lleva el foco al título y avisa cuál cumplió', async () => {
		const done = vi.fn()
		const screen = await render(RefocusHarness, { seq: 0, done })
		await new Promise(requestAnimationFrame)
		expect(document.activeElement).toBe(document.body)
		expect(done).not.toHaveBeenCalled()

		await screen.rerender({ seq: 3, done })
		await expect.poll(focusedText).toBe('De la app')
		expect(done).toHaveBeenCalledExactlyOnceWith(3)
	})
})
