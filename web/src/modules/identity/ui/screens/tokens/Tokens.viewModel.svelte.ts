import { createMutation, createQuery, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { formatDate, formatRelative } from '@/shared/ui'
import { toast } from '@/shared/ui/stores'
import { UnavailableError } from '@/shared/domain'
import { revokeTokenMutation, tokensQuery } from '../../../data'
import {
	actionAppPath,
	type ApiToken,
	type CreatedApiToken,
	isTokenExpired,
	partitionTokens,
	scopeLabel,
	TokenNotFound,
} from '../../../domain'
import { identityErrorMessage } from '../../components/identityMessages'

export interface TokenRow {
	readonly token: ApiToken
	readonly scope: string
	readonly lastUsed: string
	readonly expiry: string
	/** null: el backend no informa quién lo creó. */
	readonly creator: string | null
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

function creatorText(token: ApiToken): string | null {
	switch (token.createdBy.kind) {
		case 'user':
			return m.identity_tokens_created_by({ username: token.createdBy.username })
		case 'deleted':
			return m.identity_tokens_created_by_deleted()
		case 'unknown':
			return null
	}
}

// Ajustes · Tokens de API (`Ajustes-Tokens.dc.html`): la lista, crear en un modal (el
// valor en claro se muestra una sola vez, en una tarjeta arriba) y revocar con
// confirmación.
export class TokensViewModel {
	createOpen = $state(false)
	/** El último token creado, con su valor en claro. Solo vive acá hasta "Listo". */
	created = $state.raw<CreatedApiToken | null>(null)
	revoking = $state.raw<ApiToken | null>(null)

	readonly #client = useQueryClient()
	readonly #tokens = createQuery(() => tokensQuery())
	readonly #revoke = createMutation(() => revokeTokenMutation(this.#client))

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
			scope: scopeLabel(token.scope),
			lastUsed: lastUsedText(token),
			expiry: expiryText(token, now),
			creator: creatorText(token),
			isNew: token.id === newId,
		})
		const { active, expired } = partitionTokens(this.#tokens.data ?? [], now)
		return { active: active.map(toRow), expired: expired.map(toRow) }
	})

	get activeRows(): readonly TokenRow[] {
		return this.#rows.active
	}

	get expiredRows(): readonly TokenRow[] {
		return this.#rows.expired
	}

	/** El paso de GitHub Actions para el token recién creado (spec §14). */
	get snippet(): string {
		if (!this.created) return ''
		return [
			'- uses: CPU-commits/muelle/action@v1',
			'  with:',
			'    url: ${{ secrets.MUELLE_URL }}',
			'    token: ${{ secrets.MUELLE_TOKEN }}',
			`    app: ${actionAppPath(this.created.token.scope)}`,
		].join('\n')
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
		this.revoking = token
	}

	cancelRevoke = (): void => {
		if (!this.#revoke.isPending) this.revoking = null
	}

	confirmRevoke = async (): Promise<void> => {
		const token = this.revoking
		if (!token || this.#revoke.isPending) return
		try {
			await this.#revoke.mutateAsync(token.id)
			toast.success(m.identity_tokens_revoked())
		} catch (error) {
			if (error instanceof TokenNotFound) toast.info(m.identity_tokens_already_revoked())
			else toast.error(m.identity_tokens_revoke_error(), identityErrorMessage(error))
		} finally {
			if (this.created?.token.id === token.id) this.created = null
			this.revoking = null
		}
	}
}
