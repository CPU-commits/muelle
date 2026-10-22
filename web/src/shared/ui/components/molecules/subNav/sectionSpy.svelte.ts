import { untrack } from 'svelte'
import type { Attachment } from 'svelte/attachments'
import { type SectionWatch, watchSections } from './watchSections'

// La sección activa de una página con nav de secciones (SubNavLayout): la que se está leyendo
// (watchSections, con IntersectionObserver y sin escuchar el scroll), salvo que la haya
// marcado un link o un deep link:
// - un link de la nav (`select`) la marca en el acto y la deja marcada mientras dura el salto
//   (no parpadea con las secciones por las que pasa) y hasta que se scrollea a mano;
// - un deep link (`initial`) marca su sección desde el principio y la trae a la vista, aunque
//   la página la dibuje después de que SvelteKit buscó el ancla (llega con su query).
// Lo que llega a la página después (Proxy y DNS y Seguridad arriba de Registries, las listas
// con sus queries) mueve lo que se ve sin que nadie scrollee. Mientras quien lee no toque la
// página (la rueda, el dedo, una tecla, el puntero), eso no suelta la marca: su sección vuelve
// a la vista. Lo que sí la suelta es que lo que se ve cambie sin que cambie el contenido (quien
// lee scrolleó, o volvió atrás en el historial) o, después de que tocó la página, con lo que
// cambie. No se infiere nada del `scrollY`: la página de la que viene un link (Tokens) puede
// estar scrolleada hasta que SvelteKit la vuelve arriba.
// El hash de la URL no se toca: lo pone el link al hacer clic y nada más. No hay scroll suave:
// los saltos son los del navegador, instantáneos (nada que frenar con prefers-reduced-motion).

// Mientras dura un salto (del link, o de traer la sección de nuevo) no hay contra qué comparar.
const SETTLING = Symbol('settling')

export interface SectionSpyOptions {
	/** El ancla con la que se abrió la página (sin `#`), o null. Se lee una sola vez. */
	readonly initial?: string | null
}

export class SectionSpy {
	readonly #ids: readonly string[]
	#observed = $state<string | null>(null)
	#pinned = $state<string | null>(null)
	// Lo que mostraba el scroll al terminar el último salto: mientras siga igual, la marca se
	// queda.
	#baseline: string | null | typeof SETTLING = SETTLING
	// Quien lee tocó la página desde que se marcó: lo que cambie de ahí en más es cosa suya.
	#touched = false
	// La sección marcada ya estuvo en la página: si se va, es que se fue a otra.
	#arrived = false
	#settleRun = 0
	#watch: SectionWatch | null = null

	/** `ids`: las secciones de la página (sus `id`), en el orden de la nav y de la página. */
	constructor(ids: readonly string[], { initial = null }: SectionSpyOptions = {}) {
		this.#ids = ids
		if (initial !== null && ids.includes(initial)) this.#pinned = initial
	}

	/** La sección activa, o null si todavía no se llegó a ninguna (arriba de todo). */
	get current(): string | null {
		return this.#pinned ?? this.#observed
	}

	/**
	 * Se siguió en esta pestaña un link de la nav que lleva a `id` (el salto y el hash son del
	 * link). Con algo que no es una de las secciones (`null`, otra página), suelta la marca.
	 */
	select = (id: string | null): void => {
		if (id === null || !this.#ids.includes(id)) {
			this.#unpin()
			return
		}
		this.#pinned = id
		this.#touched = false
		this.#arrived = this.#watch?.element(id) !== undefined
		// En esta página salta el link; si no está (el link viene de otra), la trae al llegar.
		if (this.#arrived) this.#settle()
		else this.#hold()
	}

	/** Para `{@attach}` en el elemento que tiene las secciones adentro. */
	track: Attachment<HTMLElement> = (container) =>
		untrack(() => {
			const watch = watchSections(container, this.#ids, {
				view: () => this.#view(),
				layout: () => this.#layout(),
				input: () => this.#input(),
			})
			this.#watch = watch
			if (this.#pinned !== null) this.#hold()
			this.#layout()
			return () => {
				watch.stop()
				this.#watch = null
				this.#observed = null
			}
		})

	// El scroll movió lo que se ve.
	#view(): void {
		if (this.#watch === null) return
		this.#observed = this.#watch.inView
		this.#review()
	}

	// Cambió el contenido. Si quien lee no tocó la página, no fue él: la sección marcada vuelve
	// a la vista (la primera vez que aparece, también) y se espera a que termine el salto.
	#layout(): void {
		const watch = this.#watch
		if (watch === null) return
		this.#observed = watch.inView
		const id = this.#pinned
		if (id === null) return
		const element = watch.element(id)
		if (element === undefined) {
			if (this.#arrived) this.#unpin()
			return
		}
		this.#arrived = true
		if (this.#touched) {
			this.#review()
			return
		}
		element.scrollIntoView()
		this.#settle()
	}

	// Quien lee tocó la página: de ahí en más, lo que cambie es suyo. Si la sección todavía no
	// llegó, ya está en otra cosa: no se la trae.
	#input(): void {
		if (this.#pinned === null) return
		this.#touched = true
		if (!this.#arrived) this.#unpin()
	}

	// Su sección todavía no está: hasta que llegue, lo que se vea no la suelta.
	#hold(): void {
		this.#settleRun += 1
		this.#baseline = SETTLING
	}

	// El salto termina antes del frame siguiente: ahí se anota lo que muestra el scroll, con lo
	// que el navegador ya calculó aunque todavía no lo haya entregado.
	#settle(): void {
		this.#hold()
		const run = this.#settleRun
		requestAnimationFrame(() =>
			requestAnimationFrame(() => {
				const watch = this.#watch
				if (run !== this.#settleRun || watch === null) return
				watch.flush()
				this.#observed = watch.inView
				this.#baseline = this.#observed
			}),
		)
	}

	// ¿Sigue la marca del link? Mientras dura un salto, o mientras su sección no llegó, sí.
	#review(): void {
		if (this.#pinned === null || this.#baseline === SETTLING) return
		if (this.#observed !== this.#baseline) this.#unpin()
	}

	#unpin(): void {
		this.#pinned = null
		this.#touched = false
		this.#arrived = false
		this.#settleRun += 1
	}
}
