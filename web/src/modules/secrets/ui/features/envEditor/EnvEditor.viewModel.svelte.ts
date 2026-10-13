import {
	createMutation,
	createQuery,
	type CreateMutationResult,
	type CreateQueryResult,
	useQueryClient,
} from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import type { MenuEntry } from '@/shared/ui'
import { toast } from '@/shared/ui/stores'
import { ValidationError } from '@/shared/domain'
import { envQuery, patchEnvMutation } from '../../../data'
import {
	type EnvOwner,
	type EnvOwnerKind,
	type EnvPatch,
	type EnvVar,
	envKeyProblem,
	envKeys,
	EnvOwnerNotFound,
	envValueProblem,
	filterEnvVars,
	findEnvVar,
	InvalidEnvValue,
	normalizeEnvKey,
	type PatchEnvInput,
	sameEnv,
} from '../../../domain'
import {
	deletedKeyNote,
	keyProblemMessage,
	savedKeyNote,
	savedNote,
	secretsErrorMessage,
	valueProblemMessage,
} from '../../components/secretsMessages'

/** Lo que se está guardando: un solo PATCH a la vez. */
export type EnvAction = 'add' | 'edit' | 'secure' | 'delete' | 'import'

/**
 * Cómo terminó un PATCH: `changed` si el backend cambió algo, `unchanged` si lo aceptó
 * sin cambiar nada (el mismo valor otra vez, borrar algo que ya no estaba) y `failed` si
 * lo rechazó.
 */
export type SaveOutcome = 'changed' | 'unchanged' | 'failed'

type PatchRequest = Omit<PatchEnvInput, 'owner' | 'current'>

/** Un stage no pisa a nadie. */
const NO_KEYS: ReadonlySet<string> = envKeys([])

/**
 * Qué hacer con un error de validación (un 422 o un límite) de la acción: devuelve
 * `true` si lo mostró junto al formulario. Si no, va a un toast.
 */
type FieldErrorHandler = (message: string, error: ValidationError) => boolean

/**
 * A dónde vuelve el foco cuando se cierra un diálogo de una fila: a sus ⋯ (`key`), o al
 * título de la lista (`key: null`) si la fila ya no está. `seq` crece en cada pedido, así
 * que dos pedidos seguidos a la misma fila también mueven el foco.
 */
interface FocusRequest {
	readonly key: string | null
	readonly seq: number
}

// Las variables de un dueño (stage o app) y todo lo que las cambia: agregar, editar o
// reemplazar el valor, marcar como secreta y borrar. "Importar .env" (EnvImportViewModel)
// manda su PATCH por acá. Las dos pantallas de variables lo usan.
export class EnvEditorViewModel {
	search = $state('')
	/** Cuántos PATCH cambiaron algo desde que se abrió la pantalla (para el aviso). */
	savedChanges = $state(0)
	pendingAction = $state<EnvAction | null>(null)

	newKey = $state('')
	newValue = $state('')
	newSecret = $state(true)
	newKeyError = $state<string | null>(null)
	newValueError = $state<string | null>(null)
	newFormError = $state<string | null>(null)

	/** La variable del modal de editar (o reemplazar, si es secreta). */
	editing = $state.raw<EnvVar | null>(null)
	editValue = $state('')
	editSecret = $state(true)
	editError = $state<string | null>(null)
	/** El segundo paso de guardar una secreta sin la marca: su valor queda a la vista. */
	editConfirmingReveal = $state(false)

	confirmingDelete = $state.raw<EnvVar | null>(null)
	confirmingSecret = $state.raw<EnvVar | null>(null)

	focusRequest = $state.raw<FocusRequest | null>(null)

	readonly #owner: () => EnvOwner
	readonly #overridden: () => ReadonlySet<string>
	readonly #client = useQueryClient()
	readonly #env: CreateQueryResult<EnvVar[], Error>
	readonly #patch: CreateMutationResult<void, Error, PatchEnvInput>

	readonly visible = $derived.by(() => filterEnvVars(this.vars, this.search))

	/**
	 * `overridden`: en la app, las claves que también tiene el stage (la de la app gana).
	 * Es un getter porque las variables del stage se cargan aparte.
	 */
	constructor(owner: () => EnvOwner, overridden: () => ReadonlySet<string> = () => NO_KEYS) {
		this.#owner = owner
		this.#overridden = overridden
		this.#env = createQuery(() => envQuery(this.#owner()))
		this.#patch = createMutation(() => patchEnvMutation(this.#client))
	}

	get owner(): EnvOwner {
		return this.#owner()
	}

	get kind(): EnvOwnerKind {
		return this.owner.kind
	}

	get vars(): readonly EnvVar[] {
		return this.#env.data ?? []
	}

	get overridden(): ReadonlySet<string> {
		return this.#overridden()
	}

	get count(): number {
		return this.vars.length
	}

	get hasLoaded(): boolean {
		return this.#env.data !== undefined
	}

	get isLoading(): boolean {
		return this.#env.isPending
	}

	/** El error de la carga, solo si no hay nada que mostrar (un refetch fallido no tapa la lista). */
	get loadError(): unknown {
		return this.hasLoaded ? null : this.#env.error
	}

	get isNotFound(): boolean {
		return this.loadError instanceof EnvOwnerNotFound
	}

	get loadErrorMessage(): string {
		return secretsErrorMessage(this.loadError, this.kind)
	}

	get isSaving(): boolean {
		return this.pendingAction !== null
	}

	get isAdding(): boolean {
		return this.pendingAction === 'add'
	}

	get isFiltering(): boolean {
		return this.search.trim().length > 0
	}

	retry = (): void => {
		void this.#env.refetch()
	}

	setSearch = (value: string): void => {
		this.search = value
	}

	// --- Foco --------------------------------------------------------------------------

	/** El `seq` del pedido de foco si es para la fila `key`; si no, 0. */
	focusSeqFor = (key: string): number =>
		this.focusRequest?.key === key ? this.focusRequest.seq : 0

	/** El `seq` del pedido de foco si es para el título de la lista; si no, 0. */
	get headingFocusSeq(): number {
		return this.focusRequest && this.focusRequest.key === null ? this.focusRequest.seq : 0
	}

	#focus(key: string | null): void {
		this.focusRequest = { key, seq: (this.focusRequest?.seq ?? 0) + 1 }
	}

	// --- Agregar ---------------------------------------------------------------------

	setNewKey = (value: string): void => {
		this.newKey = value
		this.newKeyError = null
		this.newFormError = null
	}

	setNewValue = (value: string): void => {
		this.newValue = value
		this.newValueError = null
		this.newFormError = null
	}

	setNewSecret = (value: boolean): void => {
		this.newSecret = value
	}

	/** Al salir del campo: avisa el formato antes de mandar (vacía no, eso al enviar). */
	checkNewKey = (): void => {
		const key = normalizeEnvKey(this.newKey)
		if (key.length > 0) this.newKeyError = this.#newKeyMessage(key)
	}

	/** El formulario tiene un error que no es de un campo (un límite, un 422). */
	get hasNewFormError(): boolean {
		return this.newFormError !== null
	}

	submitAdd = async (): Promise<boolean> => {
		const key = normalizeEnvKey(this.newKey)
		const problem = envValueProblem(this.newValue)
		this.newKeyError = this.#newKeyMessage(key)
		this.newValueError = problem ? valueProblemMessage(problem) : null
		this.newFormError = null
		if (this.newKeyError || this.newValueError) return false

		// El valor va tal cual: con sus saltos de línea y sus espacios (una clave PEM).
		const sent = { key: this.newKey, value: this.newValue }
		const outcome = await this.#save(
			'add',
			{ set: [[key, sent.value]], secret: this.newSecret },
			(message, error) => {
				if (error instanceof InvalidEnvValue) this.newValueError = message
				else this.newFormError = message
				return true
			},
		)
		if (outcome === 'failed') return false
		toast.success(m.secrets_add_success(), savedKeyNote(this.kind, key))
		// Lo que se escribió mientras se guardaba (la siguiente variable) no se borra.
		if (this.newKey === sent.key && this.newValue === sent.value) {
			this.newKey = ''
			this.newValue = ''
			// La siguiente vuelve a ser secreta: olvidarse de marcarla expondría su valor.
			this.newSecret = true
		}
		return true
	}

	#newKeyMessage(key: string): string | null {
		const problem = envKeyProblem(key)
		if (problem) return keyProblemMessage(problem)
		return findEnvVar(this.vars, key) ? m.secrets_error_key_taken({ key }) : null
	}

	// --- Editar o reemplazar -----------------------------------------------------------

	openEdit = (envVar: EnvVar): void => {
		this.editing = envVar
		this.editValue = envVar.secret ? '' : envVar.value
		this.editSecret = envVar.secret
		this.editError = null
		this.editConfirmingReveal = false
	}

	/** Cancelar, Escape o un clic afuera: el foco vuelve a los ⋯ de la fila. */
	closeEdit = (): void => {
		if (this.pendingAction === 'edit') return
		const key = this.editing?.key
		this.#resetEdit()
		if (key) this.#focus(key)
	}

	setEditValue = (value: string): void => {
		this.editValue = value
		this.editError = null
		this.editConfirmingReveal = false
	}

	setEditSecret = (value: boolean): void => {
		this.editSecret = value
		this.editConfirmingReveal = false
	}

	get editTitle(): string {
		const envVar = this.editing
		if (!envVar) return ''
		return envVar.secret
			? m.secrets_replace_title({ key: envVar.key })
			: m.secrets_edit_title({ key: envVar.key })
	}

	/** Una no secreta que pasa a secreta: después no se va a poder leer. */
	get editMakesSecret(): boolean {
		return this.editing !== null && !this.editing.secret && this.editSecret
	}

	/** Una secreta que deja de serlo: su valor nuevo se va a poder leer en el panel. */
	get editRevealsSecret(): boolean {
		return this.editing !== null && this.editing.secret && !this.editSecret
	}

	/**
	 * De una secreta no se sabe el valor: guardar siempre la reescribe (también vacía, que
	 * el backend acepta). Si es el mismo valor, el backend no cambia nada.
	 */
	get editHasChanges(): boolean {
		const envVar = this.editing
		if (!envVar) return false
		if (envVar.secret) return true
		return this.editValue !== envVar.value || this.editSecret !== envVar.secret
	}

	submitEdit = async (): Promise<boolean> => {
		const envVar = this.editing
		if (!envVar) return false
		const problem = envValueProblem(this.editValue)
		if (problem) {
			this.editError = valueProblemMessage(problem)
			return false
		}
		if (!this.editHasChanges) {
			this.#resetEdit()
			this.#focus(envVar.key)
			return true
		}
		// Dejar a la vista el valor de una secreta tiene un segundo paso, como en el import.
		if (this.editRevealsSecret && !this.editConfirmingReveal) {
			this.editConfirmingReveal = true
			return false
		}

		const outcome = await this.#save(
			'edit',
			{ set: [[envVar.key, this.editValue]], secret: this.editSecret },
			(message) => {
				this.editError = message
				return true
			},
		)
		if (outcome === 'failed') return false
		if (outcome === 'changed') {
			toast.success(m.secrets_edit_success(), savedKeyNote(this.kind, envVar.key))
		} else {
			toast.info(
				m.secrets_nothing_changed(),
				m.secrets_nothing_changed_key({ key: envVar.key }),
			)
		}
		this.#resetEdit()
		this.#focus(envVar.key)
		return true
	}

	#resetEdit(): void {
		this.editing = null
		this.editValue = ''
		this.editError = null
		this.editConfirmingReveal = false
	}

	// --- Marcar como secreta -------------------------------------------------------------

	askSecure = (envVar: EnvVar): void => {
		if (!envVar.secret) this.confirmingSecret = envVar
	}

	cancelSecure = (): void => {
		if (this.pendingAction === 'secure') return
		const key = this.confirmingSecret?.key
		this.confirmingSecret = null
		if (key) this.#focus(key)
	}

	/**
	 * El backend no tiene "solo cambiar la marca": se reescribe el valor como secreta. El
	 * valor se lee de la lista al confirmar, no al abrir el diálogo: si mientras tanto
	 * cambió (otra pestaña, un sync-env), no se pisa con el viejo.
	 */
	confirmSecure = async (): Promise<boolean> => {
		const asked = this.confirmingSecret
		if (!asked) return false
		const envVar = findEnvVar(this.vars, asked.key)
		if (!envVar || envVar.secret) {
			this.confirmingSecret = null
			this.#focus(envVar ? envVar.key : null)
			return false
		}
		const outcome = await this.#save('secure', {
			set: [[envVar.key, envVar.value]],
			secret: true,
		})
		if (outcome === 'failed') return false
		toast.success(m.secrets_secure_success({ key: envVar.key }), savedNote(this.kind))
		this.confirmingSecret = null
		this.#focus(envVar.key)
		return true
	}

	// --- Borrar ------------------------------------------------------------------------

	askDelete = (envVar: EnvVar): void => {
		this.confirmingDelete = envVar
	}

	/** Qué pasa al borrarla: en una app que pisaba al stage, vuelve el valor del stage. */
	get deleteBody(): string {
		const key = this.confirmingDelete?.key
		if (this.kind === 'stage') return m.secrets_delete_body_stage()
		return key && this.overridden.has(key)
			? m.secrets_delete_body_override()
			: m.secrets_delete_body_app()
	}

	cancelDelete = (): void => {
		if (this.pendingAction === 'delete') return
		const key = this.confirmingDelete?.key
		this.confirmingDelete = null
		if (key) this.#focus(key)
	}

	/** La fila desaparece: el foco va al título de la lista. */
	confirmDelete = async (): Promise<boolean> => {
		const envVar = this.confirmingDelete
		if (!envVar) return false
		const outcome = await this.#save('delete', { unset: [envVar.key], secret: true })
		if (outcome === 'failed') return false
		toast.success(m.secrets_delete_success(), deletedKeyNote(this.kind, envVar.key))
		this.confirmingDelete = null
		this.#focus(null)
		return true
	}

	// --- Importar ----------------------------------------------------------------------

	/** Aplica el plan de "Importar .env". */
	applyImport = (patch: EnvPatch, onFieldError: FieldErrorHandler): Promise<SaveOutcome> =>
		this.#save(
			'import',
			{ set: patch.set, unset: patch.unset, secret: patch.secret },
			onFieldError,
		)

	// --- Fila --------------------------------------------------------------------------

	/** Las opciones (⋯) de una fila. Una secreta no se puede editar ni leer: se reemplaza. */
	menuFor = (envVar: EnvVar): MenuEntry[] => {
		const disabled = this.isSaving
		const remove: MenuEntry = {
			kind: 'action',
			id: 'delete',
			label: m.secrets_action_delete(),
			icon: 'trash',
			tone: 'danger',
			disabled,
			onSelect: () => this.askDelete(envVar),
		}
		if (envVar.secret) {
			return [
				{
					kind: 'action',
					id: 'replace',
					label: m.secrets_action_replace(),
					icon: 'refresh',
					disabled,
					onSelect: () => this.openEdit(envVar),
				},
				{ kind: 'separator', id: 'separator' },
				remove,
			]
		}
		return [
			{
				kind: 'action',
				id: 'edit',
				label: m.secrets_action_edit(),
				disabled,
				onSelect: () => this.openEdit(envVar),
			},
			{
				kind: 'action',
				id: 'secure',
				label: m.secrets_action_make_secret(),
				icon: 'lock',
				disabled,
				onSelect: () => this.askSecure(envVar),
			},
			{ kind: 'separator', id: 'separator' },
			remove,
		]
	}

	// Un PATCH a la vez. Con `current`, el caso de uso revisa los límites del dueño antes
	// de mandar. Un error de validación va al formulario si la acción lo sabe mostrar; el
	// resto (o sin formulario), a un toast. La mutación espera el refetch: comparar la
	// lista de antes con la de después dice si el backend cambió algo (no reescribe lo que
	// no cambia, ni su fecha). Si el refetch falló, se cuenta como cambio.
	async #save(
		action: EnvAction,
		request: PatchRequest,
		onFieldError?: FieldErrorHandler,
	): Promise<SaveOutcome> {
		if (this.pendingAction !== null) return 'failed'
		this.pendingAction = action
		const before = this.vars
		try {
			await this.#patch.mutateAsync({ owner: this.owner, current: before, ...request })
			const changed = this.#env.isError || !sameEnv(before, this.vars)
			if (changed) this.savedChanges += 1
			return changed ? 'changed' : 'unchanged'
		} catch (error) {
			const message = secretsErrorMessage(error, this.kind)
			if (!(error instanceof ValidationError && onFieldError?.(message, error))) {
				toast.error(message)
			}
			return 'failed'
		} finally {
			this.pendingAction = null
			// Las variables de la mutación tienen los valores en claro: no se guardan.
			this.#patch.reset()
		}
	}
}
