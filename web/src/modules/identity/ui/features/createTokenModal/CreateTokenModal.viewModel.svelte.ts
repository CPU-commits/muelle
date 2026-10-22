import { createMutation, createQuery, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { UnavailableError } from '@/shared/domain'
import { createTokenMutation, scopeProjectsQuery, scopeStagesQuery } from '../../../data'
import {
	ALL_PROJECTS,
	ALL_PROJECTS_EXPIRY_DAYS,
	type CreatedApiToken,
	type CreateTokenInput,
	createTokenProblems,
	expiryInputInDays,
	expiryInputRange,
	type ProjectRef,
	type StageRef,
	TokenScopeNotFound,
} from '../../../domain'
import {
	identityErrorMessage,
	type TokenFormField,
	tokenFieldError,
} from '../../components/identityMessages'

export interface CreateTokenModalParams {
	/** El token recién creado, con su valor en claro: lo muestra la pantalla, una vez. */
	onCreated: (created: CreatedApiToken) => void
}

type FieldErrors = Partial<Record<TokenFormField, string>>

function loadErrorMessage(error: unknown, fallback: string): string {
	return error instanceof UnavailableError ? m.common_unavailable() : fallback
}

// El modal "Crear token": nombre, proyecto (o todos los proyectos) y stage, de las listas
// del backend, si es de solo lectura y un vencimiento opcional. Se monta al abrirse, así
// cada vez empieza vacío y con las listas frescas.
export class CreateTokenModalViewModel {
	name = $state('')
	/** Un slug, `ALL_PROJECTS` o `''` (sin elegir). */
	project = $state('')
	/** `''`: todo el proyecto. */
	stage = $state('')
	/** Lo que muestra la casilla: con todos los proyectos, siempre true. */
	readOnly = $state(false)
	expiresOn = $state('')
	errors = $state<FieldErrors>({})
	formError = $state<string | null>(null)

	readonly expiryRange = expiryInputRange()

	/** Lo que la persona eligió en "Solo lectura" antes de pasar a todos los proyectos. */
	#readOnlyChoice = false
	/** El vencimiento que puso "todos los proyectos", mientras nadie lo cambie. */
	#proposedExpiry: string | null = null

	readonly #params: CreateTokenModalParams
	readonly #client = useQueryClient()
	readonly #projects = createQuery(() => scopeProjectsQuery())
	// "Todos los proyectos" no es un proyecto: no tiene stages que pedir.
	readonly #stages = createQuery(() => scopeStagesQuery(this.#stageProject))
	readonly #create = createMutation(() => createTokenMutation(this.#client))

	constructor(params: CreateTokenModalParams) {
		this.#params = params
	}

	get #stageProject(): string {
		return this.isAllProjects ? '' : this.project
	}

	get isAllProjects(): boolean {
		return this.project === ALL_PROJECTS
	}

	get projects(): readonly ProjectRef[] {
		return this.#projects.data ?? []
	}

	get isLoadingProjects(): boolean {
		return this.#projects.isPending
	}

	/** Mientras cargan. Sin proyectos (o si no cargaron) queda "todos los proyectos". */
	get projectSelectDisabled(): boolean {
		return this.isLoadingProjects
	}

	get projectsError(): string | null {
		if (!this.#projects.isError) return null
		return loadErrorMessage(this.#projects.error, m.identity_token_form_projects_error())
	}

	get hasNoProjects(): boolean {
		return this.#projects.isSuccess && this.projects.length === 0
	}

	/** El aviso de "no hay proyectos", salvo con todos los proyectos, que no los necesita. */
	get showNoProjects(): boolean {
		return this.hasNoProjects && !this.isAllProjects
	}

	get stages(): readonly StageRef[] {
		return this.#stageProject === '' ? [] : (this.#stages.data ?? [])
	}

	get isLoadingStages(): boolean {
		return this.#stageProject !== '' && this.#stages.isPending
	}

	get stageDisabled(): boolean {
		return this.#stageProject === '' || this.isLoadingStages
	}

	get stagesError(): string | null {
		if (this.#stageProject === '' || !this.#stages.isError) return null
		return loadErrorMessage(this.#stages.error, m.identity_token_form_stages_error())
	}

	/** Un token de todos los proyectos siempre es de solo lectura: la casilla no se toca. */
	get readOnlyLocked(): boolean {
		return this.isAllProjects
	}

	get isSubmitting(): boolean {
		return this.#create.isPending
	}

	get canSubmit(): boolean {
		if (this.isSubmitting) return false
		if (this.isAllProjects) return true
		return !this.hasNoProjects && this.projectsError === null
	}

	setName = (value: string): void => {
		this.name = value
		this.#clear('name')
	}

	setProject = (value: string): void => {
		const wasAll = this.isAllProjects
		this.project = value
		this.stage = ''
		this.#clear('project')
		this.#clear('stage')
		if (this.isAllProjects && !wasAll) this.#enterAllProjects()
		else if (!this.isAllProjects && wasAll) this.#leaveAllProjects()
	}

	setStage = (value: string): void => {
		this.stage = value
		this.#clear('stage')
	}

	setReadOnly = (value: boolean): void => {
		if (this.readOnlyLocked) return
		this.readOnly = value
	}

	setExpiresOn = (value: string): void => {
		this.expiresOn = value
		this.#clear('expiresOn')
	}

	retryProjects = (): void => {
		void this.#projects.refetch()
	}

	retryStages = (): void => {
		void this.#stages.refetch()
	}

	submit = async (): Promise<void> => {
		if (!this.canSubmit) return
		this.formError = null
		const input: CreateTokenInput = {
			name: this.name,
			project: this.project,
			stage: this.stage,
			readOnly: this.readOnly,
			expiresOn: this.expiresOn,
		}
		// Todos los campos mal se marcan a la vez, antes de mandar nada.
		this.errors = this.#problems(input)
		if (Object.keys(this.errors).length > 0) return
		try {
			const created = await this.#create.mutateAsync(input)
			this.#params.onCreated(created)
		} catch (error) {
			// El proyecto o el stage se borró mientras el modal estaba abierto.
			if (error instanceof TokenScopeNotFound) void this.#projects.refetch()
			const fieldError = tokenFieldError(error)
			if (fieldError) this.errors = { [fieldError.field]: fieldError.message }
			else this.formError = identityErrorMessage(error)
		} finally {
			// El valor en claro no se queda en el cache de mutaciones.
			this.#create.reset()
		}
	}

	// Todos los proyectos: de solo lectura y, si no había fecha, vence en 90 días (diseño
	// del MCP, §9.3). Una fecha que la persona ya eligió no se pisa.
	#enterAllProjects(): void {
		this.#readOnlyChoice = this.readOnly
		this.readOnly = true
		if (this.expiresOn !== '') return
		this.expiresOn = expiryInputInDays(ALL_PROJECTS_EXPIRY_DAYS)
		this.#proposedExpiry = this.expiresOn
		this.#clear('expiresOn')
	}

	// De vuelta a un proyecto: "Solo lectura" como estaba, y sin la fecha que se propuso si
	// nadie la cambió.
	#leaveAllProjects(): void {
		this.readOnly = this.#readOnlyChoice
		if (this.#proposedExpiry !== null && this.expiresOn === this.#proposedExpiry) {
			this.expiresOn = ''
			this.#clear('expiresOn')
		}
		this.#proposedExpiry = null
	}

	#problems(input: CreateTokenInput): FieldErrors {
		const errors: FieldErrors = {}
		for (const problem of createTokenProblems(input)) {
			const fieldError = tokenFieldError(problem)
			if (fieldError) errors[fieldError.field] = fieldError.message
		}
		return errors
	}

	#clear(field: TokenFormField): void {
		if (this.errors[field] === undefined) return
		const next = { ...this.errors }
		delete next[field]
		this.errors = next
	}
}
