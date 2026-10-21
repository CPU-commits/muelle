import { createContext, type Snippet, untrack } from 'svelte'

// Lo que una ruta anidada le pide al header del AppShell: un breadcrumb en lugar del
// logo y una nav propia en lugar de "Proyectos · Ajustes". El detalle de una app lo usa
// para mostrar "logo / proyecto / stage / app" y sus tabs.
export interface ShellOverride {
	readonly breadcrumb?: Snippet | undefined
	readonly nav?: Snippet | undefined
}

interface Entry {
	readonly token: symbol
	readonly override: ShellOverride
}

// Una pila: si dos layouts anidados piden cosas, gana el más interno, y al desmontarse
// vuelve lo del anterior.
//
// La verdad vive en `#entries`, un array común; `#stack` es solo lo que leen los
// getters. Al navegar de una ruta con <ShellHeader> a otra (de una app a las variables
// del stage), el nuevo hace `push` y después el viejo hace `release` desde la limpieza de
// un efecto que se está destruyendo. Ahí Svelte devuelve el `#stack` de ANTES del push:
// filtrar sobre ese valor borraba al nuevo y el header quedaba con el logo.
export class ShellSlots {
	#entries: readonly Entry[] = []
	#stack = $state.raw<readonly Entry[]>([])

	get breadcrumb(): Snippet | undefined {
		return this.#stack.findLast((entry) => entry.override.breadcrumb)?.override.breadcrumb
	}

	get nav(): Snippet | undefined {
		return this.#stack.findLast((entry) => entry.override.nav)?.override.nav
	}

	push(override: ShellOverride): symbol {
		const token = Symbol('shell-override')
		this.#publish([...this.#entries, { token, override }])
		return token
	}

	release(token: symbol): void {
		this.#publish(this.#entries.filter((entry) => entry.token !== token))
	}

	/** Si algún <ShellHeader> montado sigue pidiendo este snippet. */
	holds(snippet: Snippet): boolean {
		return this.#entries.some(
			({ override }) => override.breadcrumb === snippet || override.nav === snippet,
		)
	}

	// `untrack`: push y release se llaman desde el efecto de <ShellHeader>. Si la escritura
	// leyera algo con tracking, el efecto dependería de lo que él mismo escribe.
	#publish(entries: readonly Entry[]): void {
		this.#entries = entries
		untrack(() => {
			this.#stack = entries
		})
	}
}

export const [getShellSlots, setShellSlots] = createContext<ShellSlots>()
