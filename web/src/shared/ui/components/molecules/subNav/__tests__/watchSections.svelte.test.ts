import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { type SectionWatch, watchSections } from '../watchSections'
import {
	above,
	atBottom,
	band,
	bottom,
	crossing,
	installFakeObserver,
	mutationsSeen,
	passed,
	section,
	sectionEl,
	sectionsPage,
} from './fakeIntersectionObserver'

// Qué sección está a la vista, con un IntersectionObserver de mentira (fakeIntersectionObserver.ts).
// Corre en Chromium por el DOM, el MutationObserver, el ResizeObserver y los eventos de verdad.

const IDS = ['a', 'b', 'c'] as const

let page: ReturnType<typeof sectionsPage>
let watch: SectionWatch | null = null
const events = { view: vi.fn(), layout: vi.fn(), input: vi.fn() }

function start(): SectionWatch {
	watch = watchSections(page.container, IDS, events)
	return watch
}

/** Espera dos frames: el ResizeObserver avisa en el frame en que cambió el tamaño. */
const frames = () =>
	new Promise<void>((resolve) =>
		requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
	)

const fire = (...crossed: string[]) => band().fire(crossing(IDS, ...crossed))

describe('watchSections', () => {
	beforeEach(() => {
		installFakeObserver()
		for (const event of Object.values(events)) event.mockReset()
		page = sectionsPage(IDS)
	})

	afterEach(() => {
		watch?.stop()
		watch = null
		page.remove()
		vi.unstubAllGlobals()
	})

	it('sin ninguna sección en la franja de arriba, no hay ninguna a la vista', () => {
		const sections = start()

		fire()

		expect(sections.inView).toBeNull()
		expect(events.view).toHaveBeenCalled()
	})

	it('a la vista está la que ocupa la parte de arriba de la pantalla', () => {
		const sections = start()

		fire('a')
		expect(sections.inView).toBe('a')

		// Las dos tocan la franja: "a" viene de más arriba y termina, "b" empieza. Gana "b".
		above().fire(passed(IDS, 'a'))
		fire('a', 'b')
		expect(sections.inView).toBe('b')

		above().fire(passed(IDS, 'a', 'b'))
		fire('b')
		expect(sections.inView).toBe('b')

		// Se scrolleó para arriba.
		above().fire(passed(IDS))
		fire('a')
		expect(sections.inView).toBe('a')
	})

	it('una sección corta que empieza arriba de todo gana aunque la siguiente ya asome en la franja', () => {
		const sections = start()

		// "b" quedó a la altura de la nav (donde la deja un salto) y "c" empieza más abajo,
		// adentro de la franja: se está leyendo "b".
		above().fire(passed(IDS, 'a'))
		fire('b', 'c')
		expect(sections.inView).toBe('b')

		// Scrolleando un poco más, "b" ya empezó más arriba: ahora es "c".
		above().fire(passed(IDS, 'a', 'b'))
		expect(sections.inView).toBe('c')
	})

	it('lo que el navegador entrega junto se aplica junto (no pasa por una sección intermedia)', () => {
		const sections = start()
		above().fire(passed(IDS, 'a'))
		fire('a', 'b')
		const seen: (string | null)[] = []
		events.view.mockImplementation(() => seen.push(sections.inView))

		// El mismo frame: "b" empezó más arriba y "c" llega a la franja. El navegador llama a
		// cada observer por separado; el primero ya ve lo del otro.
		above().queue(passed(IDS, 'a', 'b'))
		fire('b', 'c')

		expect(seen).toEqual(['c'])
	})

	it('mira una franja en la parte de arriba de la pantalla y el fondo de la página', () => {
		start()

		expect(band().options.root).toBe(document)
		expect(band().options.rootMargin).toBe('0px 0px -70% 0px')
		expect([...band().targets]).toEqual(IDS.map(sectionEl))
		// Lo que ya empezó más arriba: todo lo que está por encima del borde de arriba.
		expect(above().options.root).toBe(document)
		expect(above().options.rootMargin).toMatch(/^\d+px 0px -100% 0px$/)
		expect([...above().targets]).toEqual(IDS.map(sectionEl))
		// El fondo: el documento entero adentro de la pantalla estirada hacia arriba, y un píxel
		// hacia abajo (un alto con decimales termina una fracción por debajo).
		expect(bottom().options.root).toBe(document)
		expect(bottom().options.threshold).toBe(1)
		expect(bottom().options.rootMargin).toMatch(/^\d+px 0px 1px 0px$/)
	})

	it('al fondo de la página gana la última, aunque no llegue arriba', () => {
		const sections = start()
		window.scrollTo(0, 500)

		above().fire(passed(IDS, 'a'))
		fire('a', 'b')
		expect(sections.inView).toBe('b')

		bottom().fire(atBottom(true))
		expect(sections.inView).toBe('c')

		bottom().fire(atBottom(false))
		expect(sections.inView).toBe('b')
	})

	it('una página que no scrollea no está "al fondo": manda la franja', () => {
		const sections = start()

		fire('a')
		bottom().fire(atBottom(true))

		expect(sections.inView).toBe('a')
	})

	it('observa las secciones que aparecen después y suelta las que se van (avisa con layout)', async () => {
		sectionEl('b').remove()
		const sections = start()
		expect(band().targets.size).toBe(2)
		expect(sections.element('b')).toBeUndefined()

		const late = section('b')
		sectionEl('a').after(late)
		await mutationsSeen()
		expect(band().targets.has(late)).toBe(true)
		expect(sections.element('b')).toBe(late)
		expect(events.layout).toHaveBeenCalledOnce()
		expect(events.view).not.toHaveBeenCalled()

		above().fire(passed(IDS, 'a'))
		fire('a', 'b')
		expect(sections.inView).toBe('b')

		late.remove()
		await mutationsSeen()
		expect(band().targets.has(late)).toBe(false)
		expect(sections.inView).toBe('a')
	})

	it('al fondo gana la última sección que está en la página', async () => {
		const sections = start()
		window.scrollTo(0, 500)
		sectionEl('c').remove()
		await mutationsSeen()

		bottom().fire(atBottom(true))

		expect(sections.inView).toBe('b')
	})

	it('un cambio de tamaño del contenido también avisa con layout; empezar a mirarlo, no', async () => {
		start()
		await frames()
		expect(events.layout).not.toHaveBeenCalled()

		sectionEl('a').style.height = '300px'
		await frames()

		expect(events.layout).toHaveBeenCalledOnce()
	})

	it('avisa con input cuando quien lee toca la página: la rueda, el dedo, una tecla, el puntero', () => {
		start()

		for (const type of ['wheel', 'touchmove', 'keydown', 'pointerdown']) {
			events.input.mockClear()
			document.body.dispatchEvent(new Event(type, { bubbles: true }))
			expect(events.input, type).toHaveBeenCalledOnce()
		}
		// Mover el puntero o soltar una tecla no.
		events.input.mockClear()
		for (const type of ['pointermove', 'keyup', 'scroll']) {
			document.body.dispatchEvent(new Event(type, { bubbles: true }))
		}
		expect(events.input).not.toHaveBeenCalled()
	})

	it('flush aplica lo que el navegador ya calculó y todavía no entregó, sin avisar', () => {
		const sections = start()
		fire('a')

		band().queue(crossing(IDS, 'c'))
		events.view.mockClear()
		sections.flush()

		expect(sections.inView).toBe('c')
		expect(events.view).not.toHaveBeenCalled()
	})

	it('stop deja de observar', async () => {
		const sections = start()
		const observers = [band(), above(), bottom()]

		sections.stop()
		watch = null

		expect(observers.every((observer) => observer.disconnected)).toBe(true)
		page.container.append(section('d'))
		sectionEl('a').style.height = '300px'
		document.body.dispatchEvent(new Event('wheel', { bubbles: true }))
		await mutationsSeen()
		await frames()
		for (const event of Object.values(events)) expect(event).not.toHaveBeenCalled()
		expect(observers.every((observer) => observer.targets.size === 0)).toBe(true)
	})
})
