import { describe, expect, it } from 'vitest'
import { page } from 'vitest/browser'
import { render } from 'vitest-browser-svelte'
import SubNavLayoutHarness from './SubNavLayoutHarness.svelte'

// El anillo de foco global: 2 px de outline con 2 px de separación (themes/global.css.ts).
const FOCUS_RING = 4

interface Parts {
	layout: HTMLElement
	aside: HTMLElement
	content: HTMLElement
	links: HTMLElement[]
}

async function mount(width: number): Promise<Parts & { unmount: () => Promise<void> }> {
	const screen = await render(SubNavLayoutHarness, { width })
	const nav = screen.getByRole('navigation', { name: 'Secciones' }).element() as HTMLElement
	const content = screen.getByTestId('contenido').element().parentElement as HTMLElement
	return {
		layout: content.parentElement as HTMLElement,
		aside: nav.parentElement as HTMLElement,
		content,
		links: [...nav.querySelectorAll('a')],
		unmount: screen.unmount,
	}
}

// La nav y el contenido en la misma fila: el contenido empieza a la derecha de la nav.
function sideBySide({ aside, content }: Parts): boolean {
	return content.getBoundingClientRect().left >= aside.getBoundingClientRect().right
}

describe('SubNavLayout', () => {
	it('la nav es sticky justo cuando va al lado del contenido, no cuando va arriba', async () => {
		await page.viewport(1400, 900)
		// 820 px = 180 de nav + 40 de gap + 600 de contenido: desde ahí entran en una fila.
		for (const width of [360, 700, 819, 819.5, 820, 820.5, 1000, 1200]) {
			const parts = await mount(width)
			expect(sideBySide(parts), `${width}px`).toBe(width >= 820)
			expect(getComputedStyle(parts.aside).position, `${width}px`).toBe(
				width >= 820 ? 'sticky' : 'static',
			)
			await parts.unmount()
		}
	})

	it('al costado, el look es el de siempre: la nav al borde y el contenido a 40 px', async () => {
		await page.viewport(1400, 900)
		const { layout, content, links } = await mount(1000)
		const left = layout.getBoundingClientRect().left

		// 180 px de nav más su parte (1 de 5) de los 180 que sobran: 216.
		for (const link of links) {
			const rect = link.getBoundingClientRect()
			expect(rect.left - left).toBe(0)
			expect(rect.width).toBe(216)
		}
		expect(links[0]!.getBoundingClientRect().top).toBe(layout.getBoundingClientRect().top)
		expect(content.getBoundingClientRect().left - left).toBe(256)
	})

	it('si la nav no entra en la pantalla, scrollea adentro sin recortar el anillo de foco', async () => {
		await page.viewport(1400, 200)
		const { aside, links } = await mount(1000)
		// A mitad del contenido: la nav está pegada arriba y entera en la pantalla.
		window.scrollTo(0, 1000)
		const rect = aside.getBoundingClientRect()

		expect(getComputedStyle(aside).overflowY).toBe('auto')
		expect(rect.top).toBeGreaterThanOrEqual(0)
		expect(rect.bottom).toBeLessThanOrEqual(window.innerHeight)
		expect(aside.scrollHeight).toBeGreaterThan(aside.clientHeight)

		// Lo que se ve de la nav es su padding box: el anillo de cada link entra a los costados,
		// y arriba el del primero (con la nav sin scrollear).
		const inner = {
			left: rect.left + aside.clientLeft,
			right: rect.left + aside.clientLeft + aside.clientWidth,
			top: rect.top + aside.clientTop,
		}
		for (const link of links) {
			const ring = link.getBoundingClientRect()
			expect(ring.left - FOCUS_RING).toBeGreaterThanOrEqual(inner.left)
			expect(ring.right + FOCUS_RING).toBeLessThanOrEqual(inner.right)
		}
		expect(links[0]!.getBoundingClientRect().top - FOCUS_RING).toBeGreaterThanOrEqual(inner.top)

		// Y abajo el del último, con la nav scrolleada hasta el fondo.
		aside.scrollTop = aside.scrollHeight
		const last = links.at(-1)!.getBoundingClientRect()
		expect(last.bottom + FOCUS_RING).toBeLessThanOrEqual(
			rect.top + aside.clientTop + aside.clientHeight,
		)
	})
})
