import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { type SectionWatch, watchSections } from '../watchSections'
import {
	atBottom,
	band,
	bottom,
	crossing,
	installFakeObserver,
	mutationsSeen,
	section,
	sectionEl,
	sectionsPage,
} from './fakeIntersectionObserver'

// Qué sección está a la vista, con un IntersectionObserver de mentira (fakeIntersectionObserver.ts).
// Corre en Chromium por el DOM y el MutationObserver de verdad.

const IDS = ['a', 'b', 'c'] as const

let page: ReturnType<typeof sectionsPage>
let watch: SectionWatch | null = null
const onChange = vi.fn()

function start(): SectionWatch {
	watch = watchSections(page.container, IDS, onChange)
	return watch
}

const fire = (...crossed: string[]) => band().fire(crossing(IDS, ...crossed))

describe('watchSections', () => {
	beforeEach(() => {
		installFakeObserver()
		onChange.mockClear()
		page = sectionsPage(IDS)
	})

	afterEach(() => {
		watch?.stop()
		watch = null
		page.remove()
		vi.unstubAllGlobals()
	})

	it('sin ninguna sección arriba de la línea, no hay ninguna a la vista', () => {
		const sections = start()

		fire()

		expect(sections.inView).toBeNull()
		expect(onChange).toHaveBeenCalled()
	})

	it('a la vista está la última (en el orden de la nav) que cruzó la línea de arriba', () => {
		const sections = start()

		fire('a')
		expect(sections.inView).toBe('a')

		// Las dos tocan la franja (una termina y la otra empieza): gana la de abajo.
		fire('a', 'b')
		expect(sections.inView).toBe('b')

		fire('b')
		expect(sections.inView).toBe('b')

		// Se scrolleó para arriba.
		fire('a')
		expect(sections.inView).toBe('a')
	})

	it('mira una franja en la parte de arriba de la pantalla y el fondo de la página', () => {
		start()

		expect(band().options.root).toBe(document)
		expect(band().options.rootMargin).toBe('0px 0px -70% 0px')
		expect([...band().targets]).toEqual(IDS.map(sectionEl))
		// El fondo: el documento entero adentro de la pantalla estirada hacia arriba.
		expect(bottom().options.root).toBe(document)
		expect(bottom().options.threshold).toBe(1)
		expect(bottom().options.rootMargin).toMatch(/^\d+px 0px 0px 0px$/)
	})

	it('al fondo de la página gana la última, aunque no llegue a la línea', () => {
		const sections = start()
		window.scrollTo(0, 500)

		fire('a', 'b')
		expect(sections.inView).toBe('b')

		bottom().fire(atBottom(true))
		expect(sections.inView).toBe('c')

		bottom().fire(atBottom(false))
		expect(sections.inView).toBe('b')
	})

	it('una página que no scrollea no está "al fondo": manda la línea', () => {
		const sections = start()

		fire('a')
		bottom().fire(atBottom(true))

		expect(sections.inView).toBe('a')
	})

	it('observa las secciones que aparecen después y suelta las que se van', async () => {
		sectionEl('b').remove()
		const sections = start()
		expect(band().targets.size).toBe(2)
		expect(sections.element('b')).toBeUndefined()

		const late = section('b')
		sectionEl('a').after(late)
		onChange.mockClear()
		await mutationsSeen()
		expect(band().targets.has(late)).toBe(true)
		expect(sections.element('b')).toBe(late)
		expect(onChange).toHaveBeenCalled()

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

	it('flush aplica lo que el navegador ya calculó y todavía no entregó', () => {
		const sections = start()
		fire('a')

		band().queue(crossing(IDS, 'c'))
		onChange.mockClear()
		sections.flush()

		expect(sections.inView).toBe('c')
		expect(onChange).toHaveBeenCalledOnce()
	})

	it('stop deja de observar', async () => {
		const sections = start()
		const observers = [band(), bottom()]

		sections.stop()
		watch = null

		expect(observers.every((observer) => observer.disconnected)).toBe(true)
		page.container.append(section('d'))
		onChange.mockClear()
		await mutationsSeen()
		expect(onChange).not.toHaveBeenCalled()
		expect(observers.every((observer) => observer.targets.size === 0)).toBe(true)
	})
})
