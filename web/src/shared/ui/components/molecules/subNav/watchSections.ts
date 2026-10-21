// Qué sección de una página con nav de secciones (SubNavLayout) se está leyendo, con
// IntersectionObserver y sin escuchar el scroll. Es la parte de SectionSpy que mira el DOM;
// la marca de un link y el deep link son de SectionSpy.
// - A la vista está la última (en el orden de `ids`, el de la nav y la página) cuyo borde de
//   arriba cruzó una línea al 30 % del alto de la pantalla: la que ocupa la parte de arriba.
// - Al fondo de la página gana la última, aunque sea corta y nunca llegue a la línea
//   ("Eliminar app").
// - Las secciones pueden aparecer después (cada una con su query) o irse: un
//   MutationObserver sobre el contenedor las vuelve a buscar.
// La raíz de los dos observers es el documento (`root: document`): su pantalla, la misma que
// la implícita en la app. Adentro de un iframe (el de los tests de componentes, escalado
// dentro de la página de vitest), la implícita sería la pantalla de la página de afuera.

// La franja que mira el observer: el 30 % de arriba de la pantalla. Una sección que la toca
// ya cruzó la línea (su borde de abajo).
const BAND = '0px 0px -70% 0px'
// El fondo de la página: el documento entero adentro de la pantalla estirada hacia arriba
// (lo que ya pasó cuenta como adentro), con `threshold: 1`. Es lo que avisa justo cuando el
// borde de abajo del documento llega al de la pantalla. Con un píxel de holgura abajo: con un
// alto con decimales (1889.4375 px), al fondo el documento termina una fracción de píxel por
// debajo de la pantalla y nunca entraría entero.
const ABOVE_THE_VIEWPORT = '1000000px 0px 1px 0px'

export interface SectionWatch {
	/** La sección a la vista, o null si ninguna cruzó la línea todavía (arriba de todo). */
	readonly inView: string | null
	/** El elemento de la sección `id`, si está en la página. */
	element(id: string): Element | undefined
	/** Aplica lo que el navegador ya calculó y todavía no entregó (avisa con `onChange`). */
	flush(): void
	stop(): void
}

/**
 * Busca las secciones `ids` (por su `id`) adentro de `container` y llama a `onChange` cada vez
 * que puede haber cambiado `inView` o las secciones que hay. Al empezar no avisa: el estado
 * inicial se lee de lo que devuelve.
 */
export function watchSections(
	container: HTMLElement,
	ids: readonly string[],
	onChange: () => void,
): SectionWatch {
	const elements = new Map<string, Element>()
	const idOf = new WeakMap<Element, string>()
	const crossed = new Set<string>()
	let atBottom = false
	let inView: string | null = null

	const applyBand = (entries: IntersectionObserverEntry[]) => {
		for (const entry of entries) {
			const id = idOf.get(entry.target)
			if (id === undefined || elements.get(id) !== entry.target) continue
			if (entry.isIntersecting) crossed.add(id)
			else crossed.delete(id)
		}
	}
	// Una página que no scrollea también tiene el documento entero a la vista: eso no es
	// haber bajado hasta el fondo.
	const applyBottom = (entries: IntersectionObserverEntry[]) => {
		const last = entries.at(-1)
		if (last) atBottom = last.intersectionRatio >= 1 && window.scrollY > 0
	}
	const compute = () => {
		const present = ids.filter((id) => elements.has(id))
		const last = present.at(-1) ?? null
		inView =
			atBottom && last !== null ? last : (present.findLast((id) => crossed.has(id)) ?? null)
	}

	const band = new IntersectionObserver(
		(entries) => {
			applyBand(entries)
			compute()
			onChange()
		},
		{ root: document, rootMargin: BAND },
	)
	const bottom = new IntersectionObserver(
		(entries) => {
			applyBottom(entries)
			compute()
			onChange()
		},
		{ root: document, rootMargin: ABOVE_THE_VIEWPORT, threshold: 1 },
	)
	bottom.observe(document.documentElement)

	const resolve = () => {
		for (const id of ids) {
			const element = container.querySelector(`#${CSS.escape(id)}`)
			const previous = elements.get(id)
			if (element === previous) continue
			if (previous) {
				band.unobserve(previous)
				elements.delete(id)
				crossed.delete(id)
			}
			if (element) {
				elements.set(id, element)
				idOf.set(element, id)
				band.observe(element)
			}
		}
		compute()
	}
	const mutations = new MutationObserver(() => {
		resolve()
		onChange()
	})
	mutations.observe(container, { childList: true, subtree: true })
	resolve()

	return {
		get inView() {
			return inView
		},
		element: (id) => elements.get(id),
		flush: () => {
			applyBand(band.takeRecords())
			applyBottom(bottom.takeRecords())
			compute()
			onChange()
		},
		stop: () => {
			band.disconnect()
			bottom.disconnect()
			mutations.disconnect()
		},
	}
}
