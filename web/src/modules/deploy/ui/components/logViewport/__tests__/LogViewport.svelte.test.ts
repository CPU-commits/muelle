import { describe, expect, it } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { userEvent } from 'vitest/browser'
import LogViewportHarness, { type HarnessApi } from './LogViewportHarness.svelte'

async function mounted(initial = 0): Promise<HarnessApi> {
	let api: HarnessApi | undefined
	await render(LogViewportHarness, {
		initial,
		onReady: (value: HarnessApi) => {
			api = value
		},
	})
	if (!api) throw new Error('el harness no expuso su API')
	return api
}

const nextFrame = () => new Promise((resolve) => requestAnimationFrame(() => resolve(null)))
const wait = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

function distanceToBottom(element: HTMLElement): number {
	return element.scrollHeight - element.scrollTop - element.clientHeight
}

describe('LogViewport', () => {
	it('con una línea cada 10 ms se queda abajo y no deja de seguir solo', async () => {
		const api = await mounted(50)
		await nextFrame()
		await nextFrame()

		for (let tick = 0; tick < 120; tick += 1) {
			api.push()
			await wait(10)
			expect(api.follow).toBe(true)
		}
		await nextFrame()
		await nextFrame()

		const scroller = api.scroller
		if (!scroller) throw new Error('sin scroller')
		expect(api.follow).toBe(true)
		expect(distanceToBottom(scroller)).toBeLessThanOrEqual(2)
	})

	it('subir con la rueda deja de seguir; volver al final lo reactiva', async () => {
		const api = await mounted(300)
		await nextFrame()
		await nextFrame()
		const scroller = api.scroller
		if (!scroller) throw new Error('sin scroller')

		await userEvent.hover(scroller)
		scroller.dispatchEvent(new WheelEvent('wheel', { deltaY: -200, bubbles: true }))
		scroller.scrollTop = 0
		await expect.poll(() => api.follow).toBe(false)

		// Llegan líneas: no lo baja.
		api.push(20)
		await nextFrame()
		await nextFrame()
		expect(scroller.scrollTop).toBe(0)

		// Quien lee vuelve al final.
		scroller.scrollTop = scroller.scrollHeight
		await expect.poll(() => api.follow).toBe(true)
	})

	it('"Ir al final" vuelve a seguir y baja', async () => {
		const api = await mounted(300)
		await nextFrame()
		const scroller = api.scroller
		if (!scroller) throw new Error('sin scroller')
		scroller.dispatchEvent(new WheelEvent('wheel', { deltaY: -100, bubbles: true }))
		scroller.scrollTop = 0
		await expect.poll(() => api.follow).toBe(false)

		const jump = document.querySelector<HTMLButtonElement>('button')
		jump?.click()

		await expect.poll(() => api.follow).toBe(true)
		await expect.poll(() => distanceToBottom(scroller)).toBeLessThanOrEqual(2)
	})
})
