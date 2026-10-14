import { describe, expect, it } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { userEvent } from 'vitest/browser'
import LogViewportHarness, { type HarnessApi } from './LogViewportHarness.svelte'
import RealLinesHarness, { type RealLinesApi } from './RealLinesHarness.svelte'

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

// Con las líneas de verdad (content-visibility, el corte de las largas): las que se pintan
// recién al llegar al final son más altas que su alto estimado, y el contenido crece
// después del scroll. El final tiene que quedar a la vista igual.
describe.each(['logs', 'deploys'] as const)('LogViewport con las líneas de %s', (variant) => {
	const long = (index: number) =>
		`RÁFAGA ${index} ${'lorem ipsum dolor sit amet '.repeat(8)}`.slice(0, 200)

	async function mountedReal(width = 480): Promise<RealLinesApi> {
		let api: RealLinesApi | undefined
		await render(RealLinesHarness, {
			variant,
			width,
			onReady: (value: RealLinesApi) => {
				api = value
			},
		})
		if (!api) throw new Error('el harness no expuso su API')
		return api
	}

	function lastLineVisible(scroller: HTMLElement): boolean {
		const items = scroller.querySelectorAll('li')
		const last = items[items.length - 1]
		if (!last) return false
		const line = last.getBoundingClientRect()
		const box = scroller.getBoundingClientRect()
		return line.bottom <= box.bottom + 1 && line.top >= box.top - 1
	}

	async function settle(): Promise<void> {
		for (let frame = 0; frame < 4; frame += 1) await nextFrame()
		await wait(50)
	}

	it('una ráfaga de 30 líneas de 200 caracteres deja la última a la vista', async () => {
		const api = await mountedReal()
		api.push(Array.from({ length: 200 }, (_, index) => long(index)))
		await settle()
		const scroller = api.scroller
		if (!scroller) throw new Error('sin scroller')

		for (let round = 0; round < 3; round += 1) {
			api.push(Array.from({ length: 30 }, (_, index) => long(1000 + round * 30 + index)))
			await settle()
			expect(api.follow).toBe(true)
			expect(lastLineVisible(scroller)).toBe(true)
			expect(distanceToBottom(scroller)).toBeLessThanOrEqual(2)
		}
	})

	it('una línea larga cada 50 ms sigue mostrando la última', async () => {
		const api = await mountedReal(375)
		api.push(Array.from({ length: 100 }, (_, index) => long(index)))
		await settle()
		const scroller = api.scroller
		if (!scroller) throw new Error('sin scroller')

		for (let tick = 0; tick < 30; tick += 1) {
			api.push([long(500 + tick)])
			await wait(50)
		}
		await settle()
		expect(api.follow).toBe(true)
		expect(lastLineVisible(scroller)).toBe(true)
	})

	it('quieto después de la ráfaga, la última sigue a la vista', async () => {
		const api = await mountedReal()
		api.push(Array.from({ length: 300 }, (_, index) => long(index)))
		await settle()
		const scroller = api.scroller
		if (!scroller) throw new Error('sin scroller')

		await wait(400)
		expect(lastLineVisible(scroller)).toBe(true)
		expect(distanceToBottom(scroller)).toBeLessThanOrEqual(2)
	})
})
