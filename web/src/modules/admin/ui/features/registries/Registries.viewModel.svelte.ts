import { createMutation, createQuery, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { formatDateTime, formatRelative, type MenuEntry } from '@/shared/ui'
import { toast } from '@/shared/ui/stores'
import { pendingPhase, type RoadmapPhase, UnavailableError } from '@/shared/domain'
import {
	addRegistryMutation,
	registriesQuery,
	removeRegistryMutation,
	updateRegistryMutation,
} from '../../../data'
import {
	parseRegistryCredentials,
	parseRegistryHost,
	type Registry,
	RegistryNotFound,
} from '../../../domain'
import { adminErrorMessage, adminFieldError } from '../../components/adminMessages'

/** El modal abierto: agregar uno nuevo o cambiar las credenciales de uno que ya está. */
export type RegistryDialog =
	| { readonly mode: 'add' }
	| { readonly mode: 'edit'; readonly host: string }

export type RegistryField = 'host' | 'username' | 'password'

type FieldErrors = Partial<Record<RegistryField, string>>

const REGISTRY_FIELDS: readonly string[] = ['host', 'username', 'password']

// Registries de Ajustes (Fase 2): listar, agregar, cambiar credenciales y eliminar. Contra
// un backend sin la Fase 2 la query falla con `UnavailableError` y la sección muestra
// "Llega con la Fase 2".
export class RegistriesViewModel {
	dialog = $state.raw<RegistryDialog | null>(null)
	host = $state('')
	username = $state('')
	password = $state('')
	errors = $state<FieldErrors>({})
	removing = $state<string | null>(null)
	/** Por qué falló eliminar; el diálogo sigue abierto y lo muestra adentro. */
	removeError = $state<string | null>(null)

	readonly #client = useQueryClient()
	readonly #registries = createQuery(() => registriesQuery())
	readonly #add = createMutation(() => addRegistryMutation(this.#client))
	readonly #update = createMutation(() => updateRegistryMutation(this.#client))
	readonly #remove = createMutation(() => removeRegistryMutation(this.#client))

	get isLoading(): boolean {
		return this.#registries.isPending
	}

	/** La fase que falta si el backend todavía no tiene `/registries`. */
	get pendingPhase(): RoadmapPhase | null {
		return pendingPhase(this.#registries.error)
	}

	get loadError(): string | null {
		if (!this.#registries.isError || this.pendingPhase !== null) return null
		return this.#registries.error instanceof UnavailableError
			? m.common_unavailable()
			: m.admin_registries_load_error()
	}

	get registries(): readonly Registry[] {
		return this.#registries.data ?? []
	}

	get isEmpty(): boolean {
		return this.#registries.isSuccess && this.registries.length === 0
	}

	get isSaving(): boolean {
		return this.#add.isPending || this.#update.isPending
	}

	get isRemoving(): boolean {
		return this.#remove.isPending
	}

	get dialogTitle(): string {
		const dialog = this.dialog
		if (dialog?.mode === 'edit') return m.admin_registry_form_edit_title({ host: dialog.host })
		return m.admin_registry_form_add_title()
	}

	/** "Actualizado hace 3 días": cuándo se cambiaron por última vez las credenciales. */
	updatedLabel = (registry: Registry): string =>
		m.admin_registries_updated({ when: formatRelative(registry.updatedAt) })

	/** La fecha y hora exactas, para el `title`. */
	updatedTitle = (registry: Registry): string => formatDateTime(registry.updatedAt)

	menuFor = (registry: Registry): MenuEntry[] => [
		{
			kind: 'action',
			id: 'edit',
			label: m.admin_registries_edit(),
			icon: 'lock',
			onSelect: () => this.openEdit(registry),
		},
		{ kind: 'separator', id: 'separator' },
		{
			kind: 'action',
			id: 'delete',
			label: m.admin_registries_delete(),
			icon: 'trash',
			tone: 'danger',
			onSelect: () => this.askRemove(registry.host),
		},
	]

	retry = (): void => {
		void this.#registries.refetch()
	}

	openAdd = (): void => {
		this.#reset('', '')
		this.dialog = { mode: 'add' }
	}

	openEdit = (registry: Registry): void => {
		this.#reset(registry.host, registry.username)
		this.dialog = { mode: 'edit', host: registry.host }
	}

	closeDialog = (): void => {
		if (this.isSaving) return
		this.dialog = null
		this.password = ''
	}

	setField = (field: RegistryField, value: string): void => {
		this[field] = value
		if (this.errors[field] === undefined) return
		const next = { ...this.errors }
		delete next[field]
		this.errors = next
	}

	submit = async (): Promise<void> => {
		const dialog = this.dialog
		if (!dialog || this.isSaving) return
		this.errors = {}
		const input = { host: this.host, username: this.username, password: this.password }
		// Lo que se ve sin mandar nada (host mal escrito, usuario vacío) no borra la
		// contraseña: todavía no salió.
		try {
			if (dialog.mode === 'add') parseRegistryHost(input.host)
			parseRegistryCredentials(input)
		} catch (error) {
			this.#fail(error)
			return
		}
		// Ya sale: la contraseña no se queda en ningún lado, ni en el campo ni en el cache
		// de mutaciones. Si el backend la rechaza, se escribe de nuevo.
		this.password = ''
		try {
			if (dialog.mode === 'add') {
				await this.#add.mutateAsync(input)
				toast.success(m.admin_registry_added())
			} else {
				await this.#update.mutateAsync({ ...input, host: dialog.host })
				toast.success(m.admin_registry_updated())
			}
			this.dialog = null
		} catch (error) {
			this.#fail(error)
		} finally {
			this.#add.reset()
			this.#update.reset()
		}
	}

	#fail(error: unknown): void {
		const fieldError = adminFieldError(error)
		if (fieldError && REGISTRY_FIELDS.includes(fieldError.field)) {
			this.errors = { [fieldError.field]: fieldError.message }
			return
		}
		toast.error(m.admin_save_error(), adminErrorMessage(error))
	}

	askRemove = (host: string): void => {
		this.removeError = null
		this.removing = host
	}

	cancelRemove = (): void => {
		if (this.isRemoving) return
		this.removing = null
		this.removeError = null
	}

	confirmRemove = async (): Promise<void> => {
		const host = this.removing
		if (!host || this.isRemoving) return
		try {
			await this.#remove.mutateAsync(host)
			toast.success(m.admin_registry_deleted())
		} catch (error) {
			if (!(error instanceof RegistryNotFound)) {
				this.removeError = adminErrorMessage(error)
				return
			}
			// Ya lo había borrado otra persona: la lista se recarga sola.
			toast.info(adminErrorMessage(error))
		}
		this.removing = null
	}

	#reset(host: string, username: string): void {
		this.host = host
		this.username = username
		this.password = ''
		this.errors = {}
	}
}
