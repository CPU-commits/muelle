import { createInfiniteQuery, createQuery, useQueryClient } from '@tanstack/svelte-query'
import { tokensQuery } from '@/modules/identity'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { formatDateTime } from '@/shared/ui'
import { UnavailableError } from '@/shared/domain'
import { auditQuery, refreshAuditOnWrites } from '../../../data'
import type { AuditActor, AuditEvent } from '../../../domain'

/** Quién, como se muestra: "token" + su nombre en mono, o un texto. */
export type ActorLabel =
	| { readonly kind: 'token'; readonly name: string }
	| { readonly kind: 'text'; readonly text: string }

export interface AuditRow {
	readonly id: number
	readonly when: string
	/** Para el `datetime` del `<time>`. */
	readonly iso: string
	readonly actor: ActorLabel
	readonly action: string
	readonly target: string
}

function actorLabel(actor: AuditActor, tokenNames: Readonly<Record<number, string>>): ActorLabel {
	switch (actor.kind) {
		case 'user':
			return {
				kind: 'text',
				text:
					actor.id === null
						? m.admin_audit_actor_user_unknown()
						: m.admin_audit_actor_user({ id: actor.id }),
			}
		case 'token': {
			const name = actor.id === null ? undefined : tokenNames[actor.id]
			if (name) return { kind: 'token', name }
			return {
				kind: 'text',
				text:
					actor.id === null
						? m.admin_audit_actor_token()
						: m.admin_audit_actor_token_id({ id: actor.id }),
			}
		}
		case 'system':
			return { kind: 'text', text: m.admin_audit_actor_system() }
		case 'other':
			return { kind: 'text', text: actor.type }
	}
}

function toRow(event: AuditEvent, tokenNames: Readonly<Record<number, string>>): AuditRow {
	return {
		id: event.id,
		when: formatDateTime(event.at),
		iso: event.at.toISOString(),
		actor: actorLabel(event.actor, tokenNames),
		action: event.action,
		target: event.target,
	}
}

// La auditoría de Ajustes: quién, qué, sobre qué y cuándo, de la más nueva a la más vieja,
// de a una página con "Cargar más" (`before`). Nunca muestra los `details`. El nombre de un
// token sale de la lista de tokens de identity mientras el token exista.
export class AuditLogViewModel {
	readonly #client = useQueryClient()
	readonly #audit = createInfiniteQuery(() => auditQuery())
	readonly #tokens = createQuery(() => tokensQuery())

	constructor() {
		// Mientras la sección está montada, lo que se hace en el panel aparece arriba.
		$effect(() => refreshAuditOnWrites(this.#client))
	}

	readonly #tokenNames = $derived(
		Object.fromEntries((this.#tokens.data ?? []).map((token) => [token.id, token.name])),
	)

	readonly rows = $derived(
		(this.#audit.data?.pages ?? []).flatMap((page) =>
			page.events.map((event) => toRow(event, this.#tokenNames)),
		),
	)

	get isLoading(): boolean {
		return this.#audit.isPending
	}

	get loadError(): string | null {
		if (!this.#audit.isError || this.#audit.isFetchNextPageError) return null
		return this.#audit.error instanceof UnavailableError
			? m.common_unavailable()
			: m.admin_audit_load_error()
	}

	get isEmpty(): boolean {
		return this.#audit.isSuccess && this.rows.length === 0
	}

	get hasMore(): boolean {
		return this.#audit.hasNextPage
	}

	get isLoadingMore(): boolean {
		return this.#audit.isFetchingNextPage
	}

	get moreError(): string | null {
		return this.#audit.isFetchNextPageError ? m.admin_audit_more_error() : null
	}

	get isRefreshing(): boolean {
		return this.#audit.isRefetching && !this.#audit.isFetchingNextPage
	}

	loadMore = (): void => {
		if (!this.#audit.hasNextPage || this.#audit.isFetchingNextPage) return
		void this.#audit.fetchNextPage()
	}

	refresh = (): void => {
		void this.#audit.refetch()
	}
}
