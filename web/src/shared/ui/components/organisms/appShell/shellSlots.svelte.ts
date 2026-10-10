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
export class ShellSlots {
	#stack = $state.raw<readonly Entry[]>([])

	get breadcrumb(): Snippet | undefined {
		return this.#stack.findLast((entry) => entry.override.breadcrumb)?.override.breadcrumb
	}

	get nav(): Snippet | undefined {
		return this.#stack.findLast((entry) => entry.override.nav)?.override.nav
	}

	// `untrack`: se llaman desde el efecto de <ShellHeader>. Si leyeran la pila con
	// tracking, el efecto dependería de lo que él mismo escribe y se repetiría sin fin.
	push(override: ShellOverride): symbol {
		const token = Symbol('shell-override')
		untrack(() => {
			this.#stack = [...this.#stack, { token, override }]
		})
		return token
	}

	release(token: symbol): void {
		untrack(() => {
			this.#stack = this.#stack.filter((entry) => entry.token !== token)
		})
	}
}

export const [getShellSlots, setShellSlots] = createContext<ShellSlots>()
