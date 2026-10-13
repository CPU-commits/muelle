import { createForm, revalidateLogic } from '@tanstack/svelte-form'
import { createMutation, useQueryClient } from '@tanstack/svelte-query'
import { untrack } from 'svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { ValidationError } from '@/shared/domain'
import { catalogKeys, updateAppMutation } from '../../../data'
import { appRefOf, KindMismatch, type ServiceApp } from '../../../domain'
import {
	catalogErrorMessage,
	serverFieldMessage,
	specMessages,
} from '../../components/catalogMessages'
import {
	type CardId,
	cardOf,
	checkSection,
	type SectionDef,
	type SectionMessages,
	sectionChanges,
	valueAt,
	visibleFieldOf,
} from './sections'

export const sectionMessages: SectionMessages = {
	...specMessages,
	unclosedQuote: () => m.catalog_error_unclosed_quote(),
	deployTimeout: () => m.catalog_error_timeout(),
}

const CARD_TITLES: Record<CardId, () => string> = {
	image: () => m.catalog_config_image(),
	run: () => m.catalog_config_run(),
	health: () => m.catalog_config_health(),
	volumes: () => m.catalog_config_volumes(),
	resources: () => m.catalog_config_resources(),
}

// Un valor de formulario comparable: los campos son texto, salvo la lista de volúmenes.
function sameValue(a: unknown, b: unknown): boolean {
	return a === b || JSON.stringify(a) === JSON.stringify(b)
}

interface ServerError {
	readonly message: string
	/** El valor que tenía el campo cuando el backend lo rechazó. */
	readonly value: unknown
}

/**
 * Guarda una tarjeta de Configuración: arma el spec completo (el guardado con lo de la
 * tarjeta encima), manda solo lo que cambió y pone los 422 del backend en su campo.
 */
export class SectionSaver<V> {
	formError = $state<string | null>(null)
	#serverErrors = $state.raw<Readonly<Record<string, ServerError>>>({})

	readonly #app: () => ServiceApp
	readonly #def: SectionDef<V>
	readonly #client = useQueryClient()
	readonly #update = createMutation(() => updateAppMutation(this.#client))

	constructor(app: () => ServiceApp, def: SectionDef<V>) {
		this.#app = app
		this.#def = def
	}

	get isPending(): boolean {
		return this.#update.isPending
	}

	/** Para el validador del formulario: `{ fields }` con los errores, o nada. */
	validate = (values: V): { fields: Record<string, string> } | undefined => {
		const { fields, elsewhere } = checkSection(this.#def, values, this.#app(), sectionMessages)
		const [problem] = elsewhere
		const card = problem ? cardOf(problem.field) : null
		this.formError = problem
			? m.catalog_config_stored_problem({
					card: card ? CARD_TITLES[card]() : m.catalog_config_other_card(),
					message: problem.message,
				})
			: null
		return Object.keys(fields).length > 0 ? { fields: { ...fields } } : undefined
	}

	/**
	 * El error del backend para un campo, mientras siga con el valor que se rechazó: al
	 * editarlo, desaparece.
	 */
	serverError = (field: string, value: unknown): string | null => {
		const error = this.#serverErrors[field]
		return error && sameValue(error.value, value) ? error.message : null
	}

	/** Devuelve la app guardada, o null si no hubo nada que guardar o falló. */
	save = async (values: V): Promise<ServiceApp | null> => {
		this.formError = null
		this.#serverErrors = {}
		const app = this.#app()
		const changes = sectionChanges(this.#def, values, app, sectionMessages)
		if (!changes) {
			toast.info(m.catalog_config_no_changes())
			return null
		}
		try {
			const saved = await this.#update.mutateAsync({ ref: appRefOf(app), changes })
			toast.success(m.catalog_config_saved(), m.catalog_config_saved_body())
			return saved.kind === 'service' ? saved : null
		} catch (error) {
			const field =
				error instanceof ValidationError && error.field
					? visibleFieldOf(this.#def, error.field, values)
					: null
			// Cada campo que nombra `fieldOf` tiene dónde mostrarse en su tarjeta (lo prueba
			// ServiceConfig.svelte.test.ts); el resto (otra tarjeta, `spec`) va arriba.
			if (field !== null && error instanceof ValidationError && error.field) {
				this.#serverErrors = {
					[field]: {
						message: serverFieldMessage(error.field, error.message),
						value: valueAt(values, field),
					},
				}
				return null
			}
			// La app es compose ahora (otra pestaña): se vuelve a cargar y la pantalla cambia
			// sola al editor de YAML.
			if (error instanceof KindMismatch) {
				void this.#client.invalidateQueries({ queryKey: catalogKeys.app(appRefOf(app)) })
			}
			this.formError = catalogErrorMessage(error)
			return null
		}
	}
}

/**
 * El formulario de una tarjeta, con las mismas reglas que el backend. Valida al guardar
 * y, después, en cada cambio (`revalidateLogic`). No al salir de un campo: un error que
 * aparece o desaparece con el blur corre el botón "Guardar" entre el mousedown y el
 * mouseup, y el clic se pierde.
 */
export function createSectionForm<V>(app: () => ServiceApp, def: SectionDef<V>) {
	const saver = new SectionSaver(app, def)
	const form = createForm(() => ({
		defaultValues: untrack(() => def.values(app())),
		validationLogic: revalidateLogic(),
		validators: {
			onDynamic: ({ value }: { value: V }) => saver.validate(value),
		},
		onSubmit: async ({ value }: { value: V }) => {
			const saved = await saver.save(value)
			if (saved) form.reset(def.values(saved))
		},
	}))
	return { saver, form }
}

export type SectionForm<V> = ReturnType<typeof createSectionForm<V>>
