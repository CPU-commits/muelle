import {
	createMutation,
	createQuery,
	type CreateQueryResult,
	useQueryClient,
} from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import { pendingPhase, type RoadmapPhase, ValidationError } from '@/shared/domain'
import {
	addDomainMutation,
	adoptRecordMutation,
	changeTargetMutation,
	domainsQuery,
	removeDomainMutation,
	retryDomainMutation,
	setProxiedMutation,
} from '../../../data'
import {
	type AppDomain,
	type AppRef,
	DomainNotFound,
	failingDomains,
	formatTarget,
	HostnameTaken,
	hostnameProblem,
	suggestedTargets,
	targetProblem,
} from '../../../domain'
import {
	hostnameProblemMessage,
	ingressErrorMessage,
	sentence,
	targetProblemMessage,
} from '../../components/labels/domainLabels'

export interface DomainsProps {
	readonly project: string
	readonly stage: string
	readonly app: string
	/**
	 * Del catálogo (lo conecta F2): los destinos posibles ("api:3000" de una app de
	 * formulario; un "servicio:puerto" por servicio de una compose). Sin ellos, el
	 * formulario sugiere los que ya usan los dominios de la app.
	 */
	readonly targets?: readonly string[]
}

// Dominios (App-Dominios.dc.html): agregar, cambiar el destino, Proxied, adoptar un
// registro existente, reintentar y eliminar. Todo se aplica al instante (sin deploy).
export class DomainsViewModel {
	hostname = $state('')
	target = $state('')
	/** Proxied al crear: apagado por defecto, como en el spec (solo cuenta en una zona de Cloudflare). */
	proxied = $state(false)
	hostnameError = $state<string | null>(null)
	targetError = $state<string | null>(null)
	formError = $state<string | null>(null)

	/** El dominio cuyo destino se está editando (el modal), o null. */
	editing = $state.raw<AppDomain | null>(null)
	editTarget = $state('')
	editError = $state<string | null>(null)

	/** El dominio a punto de borrarse (el ConfirmDialog), o null. */
	removing = $state.raw<AppDomain | null>(null)
	/** Por qué no se pudo borrar: se muestra dentro del diálogo, que queda abierto. */
	removeError = $state<string | null>(null)

	readonly #props: () => DomainsProps
	readonly #client = useQueryClient()
	/** Si quien usa el formulario ya escribió un destino, no se le pisa con la sugerencia. */
	#targetTouched = $state(false)

	readonly domains: CreateQueryResult<readonly AppDomain[], Error>
	readonly #add = createMutation(() => addDomainMutation(this.#client))
	readonly #proxied = createMutation(() => setProxiedMutation(this.#client))
	readonly #changeTarget = createMutation(() => changeTargetMutation(this.#client))
	readonly #remove = createMutation(() => removeDomainMutation(this.#client))
	readonly #adopt = createMutation(() => adoptRecordMutation(this.#client))
	readonly #retry = createMutation(() => retryDomainMutation(this.#client))

	constructor(props: () => DomainsProps) {
		this.#props = props
		// Se crea acá: sus opciones leen `#props`, que un inicializador de campo no tendría.
		this.domains = createQuery(() => domainsQuery(this.ref))
	}

	get ref(): AppRef {
		const { project, stage, app } = this.#props()
		return { project, stage, app }
	}

	get pendingPhase(): RoadmapPhase | null {
		return pendingPhase(this.domains.error)
	}

	get isLoading(): boolean {
		return this.domains.isPending
	}

	get loadError(): unknown {
		return this.pendingPhase === null ? this.domains.error : null
	}

	get list(): readonly AppDomain[] {
		return this.domains.data ?? []
	}

	get isEmpty(): boolean {
		return this.domains.isSuccess && this.list.length === 0
	}

	get suggestions(): string[] {
		return suggestedTargets(this.list, this.#props().targets ?? [])
	}

	/** Lo que se ve en el campo de destino: lo escrito, o la primera sugerencia. */
	get targetValue(): string {
		return this.#targetTouched ? this.target : (this.suggestions[0] ?? this.target)
	}

	get targetPlaceholder(): string {
		return m.ingress_target_placeholder({ service: this.ref.app })
	}

	get isAdding(): boolean {
		return this.#add.isPending
	}

	get isSavingEdit(): boolean {
		return this.#changeTarget.isPending
	}

	get isRemoving(): boolean {
		return this.#remove.isPending
	}

	/**
	 * Para el lector de pantalla, en una sola región (no una por tarjeta): qué dominios
	 * tienen error. Cambia solo si cambia ese conjunto, no con cada refresco.
	 */
	get failingAnnouncement(): string {
		const failing = failingDomains(this.list)
		if (failing.length === 0) return ''
		return m.ingress_failing_announce({
			count: failing.length,
			hostnames: failing.map((domain) => domain.hostname).join(', '),
		})
	}

	/** Hay una acción en curso sobre este dominio (para deshabilitar sus botones). */
	isBusy = (domain: AppDomain): boolean => {
		const busy = (mutation: {
			isPending: boolean
			variables?: { hostname: string } | undefined
		}) => mutation.isPending && mutation.variables?.hostname === domain.hostname
		return (
			busy(this.#proxied) ||
			busy(this.#changeTarget) ||
			busy(this.#remove) ||
			busy(this.#adopt) ||
			busy(this.#retry)
		)
	}

	setHostname = (value: string): void => {
		this.hostname = value
		this.hostnameError = null
		this.formError = null
	}

	setProxied = (value: boolean): void => {
		this.proxied = value
	}

	setTarget = (value: string): void => {
		this.#targetTouched = true
		this.target = value
		this.targetError = null
		this.formError = null
	}

	retryLoad = (): void => {
		void this.domains.refetch()
	}

	add = async (): Promise<void> => {
		if (this.#add.isPending) return
		const target = this.targetValue
		const hostnameIssue = hostnameProblem(this.hostname)
		const targetIssue = targetProblem(target)
		this.hostnameError = hostnameIssue ? hostnameProblemMessage(hostnameIssue) : null
		this.targetError = targetIssue ? targetProblemMessage(targetIssue) : null
		this.formError = null
		if (this.hostnameError || this.targetError) return
		try {
			const domain = await this.#add.mutateAsync({
				app: this.ref,
				hostname: this.hostname,
				target,
				proxied: this.proxied,
			})
			toast.success(
				m.ingress_toast_added({ hostname: domain.hostname }),
				m.ingress_toast_added_body(),
			)
			this.hostname = ''
		} catch (error) {
			if (error instanceof HostnameTaken) this.hostnameError = m.ingress_error_taken()
			else if (error instanceof ValidationError && error.field === 'hostname') {
				this.hostnameError = sentence(error.message)
			} else if (error instanceof ValidationError && error.field === 'target') {
				this.targetError = sentence(error.message)
			} else this.formError = ingressErrorMessage(error)
		}
	}

	toggleProxied = async (domain: AppDomain, proxied: boolean): Promise<void> => {
		try {
			await this.#proxied.mutateAsync({ app: this.ref, hostname: domain.hostname, proxied })
		} catch (error) {
			this.#toastError(error)
		}
	}

	adopt = async (domain: AppDomain): Promise<void> => {
		try {
			await this.#adopt.mutateAsync({ app: this.ref, hostname: domain.hostname })
			toast.success(
				m.ingress_toast_adopted(),
				m.ingress_toast_adopted_body({ hostname: domain.hostname }),
			)
		} catch (error) {
			this.#toastError(error)
		}
	}

	retry = async (domain: AppDomain): Promise<void> => {
		try {
			await this.#retry.mutateAsync({ app: this.ref, hostname: domain.hostname })
			toast.info(m.ingress_toast_retried({ hostname: domain.hostname }))
		} catch (error) {
			this.#toastError(error)
		}
	}

	openEdit = (domain: AppDomain): void => {
		this.editing = domain
		this.editTarget = formatTarget(domain.target)
		this.editError = null
	}

	setEditTarget = (value: string): void => {
		this.editTarget = value
		this.editError = null
	}

	closeEdit = (): void => {
		if (this.#changeTarget.isPending) return
		this.editing = null
	}

	saveEdit = async (): Promise<void> => {
		const domain = this.editing
		if (!domain || this.#changeTarget.isPending) return
		const issue = targetProblem(this.editTarget)
		this.editError = issue ? targetProblemMessage(issue) : null
		if (this.editError) return
		try {
			await this.#changeTarget.mutateAsync({
				app: this.ref,
				hostname: domain.hostname,
				target: this.editTarget,
			})
			toast.success(m.ingress_toast_saved({ hostname: domain.hostname }))
			this.editing = null
		} catch (error) {
			this.editError = ingressErrorMessage(error)
		}
	}

	askRemove = (domain: AppDomain): void => {
		this.removing = domain
		this.removeError = null
	}

	cancelRemove = (): void => {
		if (this.#remove.isPending) return
		this.removing = null
		this.removeError = null
	}

	confirmRemove = async (): Promise<void> => {
		const domain = this.removing
		if (!domain || this.#remove.isPending) return
		this.removeError = null
		try {
			await this.#remove.mutateAsync({ app: this.ref, hostname: domain.hostname })
			toast.success(m.ingress_toast_removed({ hostname: domain.hostname }))
			this.removing = null
		} catch (error) {
			if (error instanceof DomainNotFound) {
				// Otra persona ya lo borró: lo que se quería. Se cierra y la lista se pide de nuevo.
				this.removing = null
				void this.domains.refetch()
				return
			}
			this.removeError = ingressErrorMessage(error)
		}
	}

	#toastError(error: unknown): void {
		toast.error(m.ingress_toast_error(), ingressErrorMessage(error))
		// Si otra persona lo borró, la lista ya no vale.
		if (error instanceof DomainNotFound) void this.domains.refetch()
	}
}
