import { createMutation, createQuery, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { formatDate, formatRelative } from '@/shared/ui'
import { toast } from '@/shared/ui/stores'
import { UnavailableError } from '@/shared/domain'
import { revokeTokenMutation, tokensQuery } from '../../../data'
import {
	actionAppPath,
	type ApiToken,
	canDeploy,
	type CreatedApiToken,
	isTokenExpired,
	partitionTokens,
	scopeLabel,
	TokenNotFound,
} from '../../../domain'
import { identityErrorMessage } from '../../components/identityMessages'
import { claudeAddCommand, mcpJsonSnippet, mcpServerUrl } from './claudeCode'

export interface TokenRow {
	readonly token: ApiToken
	readonly scope: string
	/** Para el badge "solo lectura". */
	readonly readOnly: boolean
	readonly lastUsed: string
	readonly expiry: string
	/** Quién lo creó ("Creado por felipe"), o que ese usuario se borró. */
	readonly creator: string
	readonly isNew: boolean
}

// El instante de la última lectura de la lista: un valor suelto, no estado reactivo.
function currentTime(): Date {
	return new Date(Date.now())
}

function lastUsedText(token: ApiToken): string {
	return token.lastUsedAt
		? m.identity_tokens_used({ when: formatRelative(token.lastUsedAt) })
		: m.identity_tokens_never_used()
}

function expiryText(token: ApiToken, now: Date): string {
	if (!token.expiresAt) return m.identity_tokens_no_expiry()
	const date = formatDate(token.expiresAt)
	return isTokenExpired(token, now)
		? m.identity_tokens_expired_on({ date })
		: m.identity_tokens_expires({ date })
}

function creatorText(token: ApiToken): string {
	return token.createdBy.kind === 'user'
		? m.identity_tokens_created_by({ username: token.createdBy.username })
		: m.identity_tokens_created_by_deleted()
}

function scopeText(token: ApiToken): string {
	return token.scope.kind === 'allProjects'
		? m.identity_tokens_scope_all()
		: scopeLabel(token.scope)
}

// Ajustes · Tokens de API (`Ajustes-Tokens.dc.html`): la lista, crear en un modal (el
// valor en claro se muestra una sola vez, en una tarjeta arriba, con cómo usarlo en GitHub
// Actions y en Claude Code) y revocar con confirmación.
export class TokensViewModel {
	createOpen = $state(false)
	/** El último token creado, con su valor en claro. Solo vive acá hasta "Listo". */
	created = $state.raw<CreatedApiToken | null>(null)
	revoking = $state.raw<ApiToken | null>(null)
	/** Por qué falló revocar; el diálogo sigue abierto y lo muestra adentro. */
	revokeError = $state<string | null>(null)

	/** El origen en que se abrió el panel (`page.url.origin`), con su puerto si lo tiene. */
	readonly #origin: () => string
	readonly #client = useQueryClient()
	readonly #tokens = createQuery(() => tokensQuery())
	readonly #revoke = createMutation(() => revokeTokenMutation(this.#client))

	constructor(origin: () => string) {
		this.#origin = origin
	}

	get isLoading(): boolean {
		return this.#tokens.isPending
	}

	get loadError(): string | null {
		if (!this.#tokens.isError) return null
		return this.#tokens.error instanceof UnavailableError
			? m.common_unavailable()
			: m.identity_tokens_load_error()
	}

	get isEmpty(): boolean {
		return this.#tokens.isSuccess && this.#tokens.data.length === 0
	}

	readonly #rows = $derived.by(() => {
		const now = currentTime()
		const newId = this.created?.token.id
		const toRow = (token: ApiToken): TokenRow => ({
			token,
			scope: scopeText(token),
			readOnly: token.readOnly,
			lastUsed: lastUsedText(token),
			expiry: expiryText(token, now),
			creator: creatorText(token),
			isNew: token.id === newId,
		})
		const { active, expired } = partitionTokens(this.#tokens.data ?? [], now)
		// El recién creado va arriba, junto a la tarjeta con su valor; el resto, como llega.
		const rows = active.map(toRow)
		const fresh = rows.filter((row) => row.isNew)
		return {
			active: [...fresh, ...rows.filter((row) => !row.isNew)],
			expired: expired.map(toRow),
		}
	})

	get activeRows(): readonly TokenRow[] {
		return this.#rows.active
	}

	get expiredRows(): readonly TokenRow[] {
		return this.#rows.expired
	}

	/**
	 * El token recién creado despliega: no es de solo lectura (uno de todos los proyectos
	 * nunca lo es). Solo entonces sirve para la GitHub Action.
	 */
	get canDeploy(): boolean {
		return this.created !== null && canDeploy(this.created.token)
	}

	/** El aviso de que Claude Code, con este token, puede desplegar y cambiar variables. */
	get showWriteWarning(): boolean {
		return this.canDeploy
	}

	/**
	 * El paso de GitHub Actions para el token recién creado (spec §14), solo si despliega.
	 * `@v0` es la versión mayor de la action, que se mueve con cada release mientras no haya
	 * 1.0.
	 */
	get snippet(): string {
		const scope = this.created?.token.scope
		if (!this.canDeploy || scope?.kind !== 'project') return ''
		return [
			'- uses: CPU-commits/muelle/action@v0',
			'  with:',
			'    url: ${{ secrets.MUELLE_URL }}',
			'    token: ${{ secrets.MUELLE_TOKEN }}',
			`    app: ${actionAppPath(scope)}`,
		].join('\n')
	}

	/** El servidor MCP en el origen en que se abrió el panel (contrato §8.7). */
	get mcpUrl(): string {
		return mcpServerUrl(this.#origin())
	}

	/** `claude mcp add` con el token recién creado: lleva el secreto, y se va con "Listo". */
	get claudeCommand(): string {
		return this.created ? claudeAddCommand(this.mcpUrl, this.created.secret) : ''
	}

	/** El `.mcp.json` para compartir en el repositorio, con `${MUELLE_TOKEN}`. */
	get mcpJson(): string {
		return this.created ? mcpJsonSnippet(this.mcpUrl) : ''
	}

	get isRevoking(): boolean {
		return this.#revoke.isPending
	}

	retry = (): void => {
		void this.#tokens.refetch()
	}

	openCreate = (): void => {
		this.createOpen = true
	}

	closeCreate = (): void => {
		this.createOpen = false
	}

	onCreated = (created: CreatedApiToken): void => {
		this.created = created
		this.createOpen = false
		toast.success(m.identity_tokens_created_toast())
	}

	dismissCreated = (): void => {
		this.created = null
	}

	askRevoke = (token: ApiToken): void => {
		this.revokeError = null
		this.revoking = token
	}

	cancelRevoke = (): void => {
		if (this.#revoke.isPending) return
		this.revoking = null
		this.revokeError = null
	}

	confirmRevoke = async (): Promise<void> => {
		const token = this.revoking
		if (!token || this.#revoke.isPending) return
		try {
			await this.#revoke.mutateAsync(token.id)
			toast.success(m.identity_tokens_revoked())
		} catch (error) {
			if (!(error instanceof TokenNotFound)) {
				this.revokeError = identityErrorMessage(error)
				return
			}
			toast.info(m.identity_tokens_already_revoked())
		}
		// Revocado (o ya lo estaba): se va de la lista y, si era el nuevo, su tarjeta también.
		if (this.created?.token.id === token.id) this.created = null
		this.revoking = null
	}
}
