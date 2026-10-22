import { createMutation, createQuery, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { UnavailableError } from '@/shared/domain'
import { createTokenMutation, scopeProjectsQuery, scopeStagesQuery } from '../../../data'
import {
	type CreatedApiToken,
	type CreateTokenInput,
	createTokenProblems,
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

// El modal "Crear token": nombre, proyecto y stage (de las listas del backend) y un
// vencimiento opcional. Se monta al abrirse, así cada vez empieza vacío y con las listas
// frescas.
export class CreateTokenModalViewModel {
	name = $state('')
	project = $state('')
	/** `''`: todo el proyecto. */
	stage = $state('')
	expiresOn = $state('')
	errors = $state<FieldErrors>({})
	formError = $state<string | null>(null)

	readonly expiryRange = expiryInputRange()

	readonly #params: CreateTokenModalParams
	readonly #client = useQueryClient()
	readonly #projects = createQuery(() => scopeProjectsQuery())
	readonly #stages = createQuery(() => scopeStagesQuery(this.project))
	readonly #create = createMutation(() => createTokenMutation(this.#client))

	constructor(params: CreateTokenModalParams) {
		this.#params = params
	}

	get projects(): readonly ProjectRef[] {
		return this.#projects.data ?? []
	}

	get isLoadingProjects(): boolean {
		return this.#projects.isPending
	}

	get projectsError(): string | null {
		if (!this.#projects.isError) return null
		return loadErrorMessage(this.#projects.error, m.identity_token_form_projects_error())
	}

	get hasNoProjects(): boolean {
		return this.#projects.isSuccess && this.projects.length === 0
	}

	get stages(): readonly StageRef[] {
		return this.project === '' ? [] : (this.#stages.data ?? [])
	}

	get isLoadingStages(): boolean {
		return this.project !== '' && this.#stages.isPending
	}

	get stagesError(): string | null {
		if (this.project === '' || !this.#stages.isError) return null
		return loadErrorMessage(this.#stages.error, m.identity_token_form_stages_error())
	}

	get isSubmitting(): boolean {
		return this.#create.isPending
	}

	get canSubmit(): boolean {
		return !this.isSubmitting && !this.hasNoProjects && this.projectsError === null
	}

	setName = (value: string): void => {
		this.name = value
		this.#clear('name')
	}

	setProject = (value: string): void => {
		this.project = value
		this.stage = ''
		this.#clear('project')
		this.#clear('stage')
	}

	setStage = (value: string): void => {
		this.stage = value
		this.#clear('stage')
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
		const input = {
			name: this.name,
			project: this.project,
			stage: this.stage,
			readOnly: false,
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
