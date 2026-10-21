import { afterEach, describe, expect, it, vi } from 'vitest'
import { page } from 'vitest/browser'
import { render } from 'vitest-browser-svelte'
import { SectionSpy } from '../sectionSpy.svelte.ts'
import SubNavLayoutHarness from './SubNavLayoutHarness.svelte'

// El anillo de foco global: 2 px de outline con 2 px de separación (themes/global.css.ts).
const FOCUS_RING = 4

interface Parts {
	layout: HTMLElement
	aside: HTMLElement
	content: HTMLElement
	links: HTMLElement[]
}

// Los altos de las tarjetas de Configuración (la última, "Eliminar app", es corta).
const CONFIG_SECTIONS = [200, 350, 420, 280, 220, 140]
const SECTION_IDS = CONFIG_SECTIONS.map((_, index) => `seccion-${index}`)

type HarnessProps = {
	sections?: readonly number[]
	spy?: SectionSpy
	onselect?: (id: string) => void
}

async function mount(
	width: number,
	props: HarnessProps = {},
): Promise<Parts & { unmount: () => Promise<void> }> {
	const screen = await render(SubNavLayoutHarness, { width, ...props })
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

function sectionTop(id: string): number {
	const element = document.getElementById(id)
	if (element === null) throw new Error(`no está #${id}`)
	return element.getBoundingClientRect().top
}

const activeLabel = (links: HTMLElement[]) =>
	links.find((link) => link.getAttribute('aria-current') === 'page')?.textContent?.trim()

describe('SubNavLayout', () => {
	afterEach(() => {
		window.scrollTo(0, 0)
	})

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

	it('al saltar a una sección, la tarjeta queda a la altura de los links de la nav pegada arriba', async () => {
		await page.viewport(1400, 600)
		const { links } = await mount(1000, { sections: CONFIG_SECTIONS })

		for (const id of ['seccion-2', 'seccion-3']) {
			document.getElementById(id)?.scrollIntoView()
			// La nav quedó pegada arriba (sticky) y la tarjeta, a su altura: 24 px del borde.
			expect(window.scrollY, id).toBeGreaterThan(0)
			expect(links[0]!.getBoundingClientRect().top, id).toBe(24)
			expect(sectionTop(id), id).toBe(24)
		}
	})

	it('un clic en un link avisa con su onselect; uno que abre otra pestaña no', async () => {
		await page.viewport(1400, 600)
		const onselect = vi.fn()
		const { links } = await mount(1000, { onselect })
		// Que ningún clic navegue de verdad (el iframe del test no es la app).
		const stay = (event: Event) => event.preventDefault()
		document.addEventListener('click', stay)

		links[3]!.click()
		expect(onselect).toHaveBeenCalledExactlyOnceWith('seccion-3')

		onselect.mockClear()
		for (const modifier of ['ctrlKey', 'metaKey', 'shiftKey', 'altKey'] as const) {
			links[4]!.dispatchEvent(
				new MouseEvent('click', { bubbles: true, cancelable: true, [modifier]: true }),
			)
		}
		links[4]!.dispatchEvent(
			new MouseEvent('click', { bubbles: true, cancelable: true, button: 1 }),
		)
		expect(onselect).not.toHaveBeenCalled()

		document.removeEventListener('click', stay)
	})

	it('con un SectionSpy, la activa sigue a la sección que se está leyendo', async () => {
		await page.viewport(1400, 600)
		const spy = new SectionSpy(SECTION_IDS)
		const { links } = await mount(1000, { sections: CONFIG_SECTIONS, spy })
		const scrollToSection = (id: string, top: number) =>
			window.scrollTo(0, window.scrollY + sectionTop(id) - top)

		// Arriba de todo, la primera (con este header corto, ya cruzó la línea: al 30 % de la
		// pantalla).
		expect(activeLabel(links)).toBe('Imagen')

		// Volúmenes arriba, en la parte de arriba de la pantalla.
		scrollToSection('seccion-3', 100)
		await expect.poll(() => activeLabel(links)).toBe('Volúmenes')

		// Recursos todavía por debajo de la línea: sigue Volúmenes.
		scrollToSection('seccion-4', 300)
		await expect.poll(() => activeLabel(links)).toBe('Volúmenes')
		scrollToSection('seccion-4', 160)
		await expect.poll(() => activeLabel(links)).toBe('Recursos')

		// Al fondo, "Eliminar app" (corta, nunca llega a la línea).
		window.scrollTo(0, document.documentElement.scrollHeight)
		await expect.poll(() => activeLabel(links)).toBe('Eliminar app')
		expect(sectionTop('seccion-5')).toBeGreaterThan(600 * 0.3)

		window.scrollTo(0, 0)
		await expect.poll(() => activeLabel(links)).toBe('Imagen')
	})

	it('con un alto de página con decimales, al fondo también gana la última', async () => {
		await page.viewport(1400, 600)
		const spy = new SectionSpy(SECTION_IDS)
		// Como Configuración sin healthcheck: al fondo, el documento termina una fracción de
		// píxel por debajo de la pantalla.
		const sections = CONFIG_SECTIONS.map((height, index) =>
			index === 2 ? height + 0.4 : height,
		)
		const { links } = await mount(1000, { sections, spy })
		expect(document.documentElement.getBoundingClientRect().height % 1).not.toBe(0)

		window.scrollTo(0, document.documentElement.scrollHeight)
		await expect.poll(() => activeLabel(links)).toBe('Eliminar app')
	})
})
