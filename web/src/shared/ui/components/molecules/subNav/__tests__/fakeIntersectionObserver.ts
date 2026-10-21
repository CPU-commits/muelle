import { vi } from 'vitest'

// Un IntersectionObserver de mentira para watchSections y SectionSpy: cada test dice qué
// sección cruzó la línea de arriba y si la página llegó al fondo, sin depender de las
// medidas del navegador. Lo de verdad (el de Chromium) lo prueban SubNavLayout.svelte.test.ts
// y e2e/subNav.e2e.ts.

type FakeEntry = { isIntersecting?: boolean; intersectionRatio?: number }

export class FakeObserver {
	static all: FakeObserver[] = []
	readonly targets = new Set<Element>()
	queued: IntersectionObserverEntry[] = []
	disconnected = false

	constructor(
		readonly callback: IntersectionObserverCallback,
		readonly options: IntersectionObserverInit = {},
	) {
		FakeObserver.all.push(this)
	}

	observe(target: Element): void {
		this.targets.add(target)
	}

	unobserve(target: Element): void {
		this.targets.delete(target)
	}

	disconnect(): void {
		this.targets.clear()
		this.disconnected = true
	}

	takeRecords(): IntersectionObserverEntry[] {
		const records = this.queued
		this.queued = []
		return records
	}

	/** Entrega entradas como lo haría el navegador después de un frame. */
	fire(entries: [Element, FakeEntry][]): void {
		this.callback(entries.map(toEntry), this as unknown as IntersectionObserver)
	}

	/** Deja entradas calculadas que el navegador todavía no entregó (una tarea larga). */
	queue(entries: [Element, FakeEntry][]): void {
		this.queued.push(...entries.map(toEntry))
	}
}

function toEntry([target, entry]: [Element, FakeEntry]): IntersectionObserverEntry {
	return {
		target,
		isIntersecting: entry.isIntersecting ?? (entry.intersectionRatio ?? 0) > 0,
		intersectionRatio: entry.intersectionRatio ?? (entry.isIntersecting ? 1 : 0),
	} as IntersectionObserverEntry
}

export function installFakeObserver(): void {
	FakeObserver.all = []
	vi.stubGlobal('IntersectionObserver', FakeObserver)
}

/** El observer de las secciones (la franja de arriba de la pantalla). */
export function band(): FakeObserver {
	const observer = FakeObserver.all.find(
		(candidate) => !candidate.disconnected && !candidate.targets.has(document.documentElement),
	)
	if (observer === undefined) throw new Error('no hay observer de secciones')
	return observer
}

/** El observer del fondo de la página (mira el documento entero). */
export function bottom(): FakeObserver {
	const observer = FakeObserver.all.find((candidate) =>
		candidate.targets.has(document.documentElement),
	)
	if (observer === undefined) throw new Error('no hay observer del fondo')
	return observer
}

/** Una entrada por cada sección de `ids` que está en la página: cruzó la línea o no. */
export function crossing(ids: readonly string[], ...crossed: string[]): [Element, FakeEntry][] {
	return ids.flatMap((id): [Element, FakeEntry][] => {
		const element = document.getElementById(id)
		return element === null ? [] : [[element, { isIntersecting: crossed.includes(id) }]]
	})
}

/** La entrada del fondo: el documento entero a la vista (llegó al fondo) o no. */
export function atBottom(yes: boolean): [Element, FakeEntry][] {
	return [[document.documentElement, { intersectionRatio: yes ? 1 : 0.8 }]]
}

/** Las secciones de los tests, en un contenedor debajo de un relleno: la página scrollea. */
export function sectionsPage(ids: readonly string[]): {
	container: HTMLElement
	remove: () => void
} {
	const spacer = document.createElement('div')
	spacer.style.height = '4000px'
	const container = document.createElement('div')
	container.append(...ids.map(section))
	document.body.append(spacer, container)
	window.scrollTo(0, 0)
	return {
		container,
		remove: () => {
			container.remove()
			spacer.remove()
			window.scrollTo(0, 0)
		},
	}
}

export function section(id: string): HTMLElement {
	const element = document.createElement('section')
	element.id = id
	return element
}

export function sectionEl(id: string): HTMLElement {
	const element = document.getElementById(id)
	if (element === null) throw new Error(`no está #${id}`)
	return element
}

/** Espera a que el MutationObserver vea lo que se agregó o se sacó. */
export const mutationsSeen = () => new Promise<void>((resolve) => setTimeout(resolve, 0))
