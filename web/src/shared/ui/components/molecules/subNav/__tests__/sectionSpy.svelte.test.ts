import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { SectionSpy } from '../sectionSpy.svelte.ts'
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

// La marca de un link y la de un deep link, sobre lo que dice watchSections (con el
// IntersectionObserver de mentira de fakeIntersectionObserver.ts).

const IDS = ['a', 'b', 'c'] as const

let page: ReturnType<typeof sectionsPage>
let stops: (() => void)[] = []

/** Monta el spy sobre el contenedor (lo que hace `{@attach spy.track}`). */
function track(spy: SectionSpy): void {
	const stop = spy.track(page.container)
	if (typeof stop === 'function') stops.push(stop)
}

const fire = (...crossed: string[]) => band().fire(crossing(IDS, ...crossed))

/** Espera a que pase el salto de un link (dos frames). */
const settled = () =>
	new Promise<void>((resolve) =>
		requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
	)

describe('SectionSpy', () => {
	beforeEach(() => {
		installFakeObserver()
		page = sectionsPage(IDS)
	})

	afterEach(() => {
		for (const stop of stops) stop()
		stops = []
		page.remove()
		vi.unstubAllGlobals()
		vi.restoreAllMocks()
	})

	it('sin link ni ancla, la activa es la sección a la vista', () => {
		const spy = new SectionSpy(IDS)
		expect(spy.current).toBeNull()
		track(spy)

		fire('a', 'b')
		expect(spy.current).toBe('b')

		window.scrollTo(0, 500)
		bottom().fire(atBottom(true))
		expect(spy.current).toBe('c')
	})

	it('elegir un link la marca enseguida y no parpadea mientras el salto pasa por otras', async () => {
		const spy = new SectionSpy(IDS)
		track(spy)
		fire('a')

		spy.select('c')
		expect(spy.current).toBe('c')

		// El salto pasa por "b"; al terminar, "c" es corta y la línea queda en "b".
		fire('b')
		expect(spy.current).toBe('c')
		await settled()
		expect(spy.current).toBe('c')

		// Algo que no cambia la sección a la vista (otra entrada igual) no la suelta.
		fire('b')
		expect(spy.current).toBe('c')

		// Scrollear a mano sí: la activa vuelve a seguir al scroll.
		fire('a')
		expect(spy.current).toBe('a')
	})

	it('el salto se mide con lo que el navegador ya calculó aunque todavía no lo entregó', async () => {
		const spy = new SectionSpy(IDS)
		track(spy)
		fire('a')

		spy.select('b')
		// El frame del salto calculó "c", pero la entrada llega tarde (una tarea larga).
		band().queue(crossing(IDS, 'c'))
		await settled()
		expect(spy.current).toBe('b')

		fire('c')
		expect(spy.current).toBe('b')
		fire('a')
		expect(spy.current).toBe('a')
	})

	it('al fondo, elegir una sección que no es la última la deja marcada', async () => {
		const spy = new SectionSpy(IDS)
		track(spy)
		window.scrollTo(0, 500)
		fire('b')
		bottom().fire(atBottom(true))
		expect(spy.current).toBe('c')

		spy.select('b')
		await settled()
		expect(spy.current).toBe('b')

		// Subir un poco: deja de estar al fondo y sigue al scroll.
		fire('a')
		bottom().fire(atBottom(false))
		expect(spy.current).toBe('a')
	})

	it('elegir un link que no es una sección suelta la marca', () => {
		const spy = new SectionSpy(IDS)
		track(spy)
		fire('a')

		spy.select('c')
		spy.select(null)
		expect(spy.current).toBe('a')

		spy.select('c')
		spy.select('otra')
		expect(spy.current).toBe('a')
	})

	it('elegir una sección que todavía no está (viene de otra página) la trae cuando aparece', async () => {
		sectionEl('c').remove()
		const spy = new SectionSpy(IDS)
		track(spy)

		spy.select('c')
		expect(spy.current).toBe('c')
		// Mientras carga, lo que pasa arriba (con la página sin scrollear) no la suelta.
		fire('a')
		expect(spy.current).toBe('c')

		const late = section('c')
		const scrollIntoView = vi.spyOn(late, 'scrollIntoView')
		page.container.append(late)
		await mutationsSeen()

		expect(scrollIntoView).toHaveBeenCalledOnce()
		await settled()
		expect(spy.current).toBe('c')
	})

	it('un deep link marca su sección desde el principio y la trae a la vista', () => {
		const spy = new SectionSpy(IDS, { initial: 'b' })
		expect(spy.current).toBe('b')

		const scrollIntoView = vi.spyOn(sectionEl('b'), 'scrollIntoView')
		track(spy)

		expect(scrollIntoView).toHaveBeenCalledOnce()
		fire('b', 'c')
		expect(spy.current).toBe('b')
	})

	it('si la sección del deep link aparece después (carga aparte), la trae cuando aparece', async () => {
		sectionEl('b').remove()
		const spy = new SectionSpy(IDS, { initial: 'b' })
		track(spy)
		fire()
		expect(spy.current).toBe('b')

		const late = section('b')
		const scrollIntoView = vi.spyOn(late, 'scrollIntoView')
		sectionEl('a').after(late)
		await mutationsSeen()

		expect(scrollIntoView).toHaveBeenCalledOnce()
		expect(spy.current).toBe('b')
	})

	it('si se scrolleó antes de que aparezca la sección del deep link, no salta y la suelta', async () => {
		sectionEl('b').remove()
		const spy = new SectionSpy(IDS, { initial: 'b' })
		track(spy)

		window.scrollTo(0, 300)
		fire('a')
		expect(spy.current).toBe('a')

		const late = section('b')
		const scrollIntoView = vi.spyOn(late, 'scrollIntoView')
		sectionEl('a').after(late)
		await mutationsSeen()

		expect(scrollIntoView).not.toHaveBeenCalled()
		expect(spy.current).toBe('a')
	})

	it('con la página ya scrolleada al montar (un F5 que recuperó la posición), no salta', () => {
		window.scrollTo(0, 300)
		const spy = new SectionSpy(IDS, { initial: 'b' })
		const scrollIntoView = vi.spyOn(sectionEl('b'), 'scrollIntoView')
		track(spy)

		expect(scrollIntoView).not.toHaveBeenCalled()
		fire('a')
		expect(spy.current).toBe('a')
	})

	it('un ancla que no es una de las secciones no marca nada', () => {
		const spy = new SectionSpy(IDS, { initial: 'otra' })
		const scrollIntoView = vi.spyOn(sectionEl('a'), 'scrollIntoView')
		track(spy)

		expect(spy.current).toBeNull()
		expect(scrollIntoView).not.toHaveBeenCalled()
	})

	it('al desmontarse deja de observar y no queda ninguna activa', () => {
		const spy = new SectionSpy(IDS)
		track(spy)
		const observers = [band(), bottom()]
		fire('a')

		for (const stop of stops) stop()
		stops = []

		expect(observers.every((observer) => observer.disconnected)).toBe(true)
		expect(spy.current).toBeNull()
	})
})
