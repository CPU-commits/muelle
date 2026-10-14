import { createQuery } from '@tanstack/svelte-query'
import { appQuery } from '../../../data'
import { type App, AppNotFound, type AppRef, type AppSummary, appSummaryOf } from '../../../domain'
import { catalogErrorMessage } from '../../components/catalogMessages'

// App·Configuración: carga la app y elige el formulario (servicio) o el editor de YAML
// (compose). Cada uno guarda por su cuenta.
export class AppConfigViewModel {
	readonly #ref: () => AppRef
	readonly #app

	constructor(ref: () => AppRef) {
		this.#ref = ref
		// La query lee la ref al crearse: va después de guardarla.
		this.#app = createQuery(() => appQuery(this.#ref()))
	}

	get app(): App | undefined {
		return this.#app.data
	}

	get summary(): AppSummary | undefined {
		const app = this.#app.data
		return app ? appSummaryOf(app) : undefined
	}

	/** Para el `{#key}`: otra app es otro formulario, con sus propios valores iniciales. */
	get key(): string {
		const ref = this.#ref()
		return `${ref.project}/${ref.stage}/${ref.app}`
	}

	get isLoading(): boolean {
		return this.#app.isPending
	}

	get isNotFound(): boolean {
		return this.#app.error instanceof AppNotFound
	}

	get loadError(): string | null {
		const error = this.#app.error
		if (!error || this.isNotFound) return null
		return catalogErrorMessage(error)
	}

	retry = (): void => {
		void this.#app.refetch()
	}
}
