import { createMutation, createQuery, useQueryClient } from '@tanstack/svelte-query'
import { meQuery } from '@/modules/auth'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { formatDate, formatDateTime, type MenuEntry } from '@/shared/ui'
import { toast } from '@/shared/ui/stores'
import { UnavailableError } from '@/shared/domain'
import {
	createInviteMutation,
	deleteUserMutation,
	resetUserTotpMutation,
	usersQuery,
} from '../../../data'
import { type Invite, inviteHoursLeft, isCurrentUser, type User } from '../../../domain'
import { identityErrorMessage } from '../../components/identityMessages'

/** Lo que espera confirmación: borrar a alguien o quitarle el 2FA. */
export interface PendingUserAction {
	readonly kind: 'delete' | 'resetTotp'
	readonly username: string
}

export interface UserRow {
	readonly user: User
	readonly isYou: boolean
	readonly since: string
	/** Vacío en la fila propia: a sí mismo no se borra ni se quita el 2FA desde acá. */
	readonly menu: readonly MenuEntry[]
}

// Usuarios de Ajustes: la lista, invitar (la URL se muestra una vez) y, para los demás,
// quitar el 2FA o eliminar, siempre con confirmación.
export class UsersSectionViewModel {
	/** La invitación recién creada. Solo vive acá, mientras el modal está abierto. */
	invite = $state.raw<Invite | null>(null)
	pending = $state.raw<PendingUserAction | null>(null)

	readonly #client = useQueryClient()
	readonly #users = createQuery(() => usersQuery())
	readonly #me = createQuery(() => meQuery())
	readonly #createInvite = createMutation(() => createInviteMutation())
	readonly #delete = createMutation(() => deleteUserMutation(this.#client))
	readonly #resetTotp = createMutation(() => resetUserTotpMutation(this.#client))

	get isLoading(): boolean {
		return this.#users.isPending
	}

	get loadError(): string | null {
		if (!this.#users.isError) return null
		return this.#users.error instanceof UnavailableError
			? m.common_unavailable()
			: m.identity_users_load_error()
	}

	get currentUsername(): string | null {
		return this.#me.data?.username ?? null
	}

	get rows(): readonly UserRow[] {
		const current = this.currentUsername
		return (this.#users.data ?? []).map((user) => {
			const isYou = isCurrentUser(user, current)
			return {
				user,
				isYou,
				since: m.identity_users_since({ date: formatDate(user.createdAt) }),
				menu: isYou ? [] : this.#menuFor(user),
			}
		})
	}

	#menuFor(user: User): MenuEntry[] {
		const remove: MenuEntry = {
			kind: 'action',
			id: 'delete',
			label: m.identity_users_delete(),
			icon: 'trash',
			tone: 'danger',
			onSelect: () => this.askDelete(user.username),
		}
		if (!user.totpEnabled) return [remove]
		return [
			{
				kind: 'action',
				id: 'reset-totp',
				label: m.identity_users_reset_totp(),
				icon: 'shield',
				onSelect: () => this.askResetTotp(user.username),
			},
			{ kind: 'separator', id: 'separator' },
			remove,
		]
	}

	get isInviting(): boolean {
		return this.#createInvite.isPending
	}

	get inviteExpiry(): string {
		if (!this.invite) return ''
		return m.identity_invite_expires({
			hours: inviteHoursLeft(this.invite),
			date: formatDateTime(this.invite.expiresAt),
		})
	}

	get isConfirming(): boolean {
		return this.#delete.isPending || this.#resetTotp.isPending
	}

	get confirmTitle(): string {
		const pending = this.pending
		if (!pending) return ''
		return pending.kind === 'delete'
			? m.identity_users_delete_title({ username: pending.username })
			: m.identity_users_reset_title({ username: pending.username })
	}

	get confirmBody(): string {
		return this.pending?.kind === 'resetTotp'
			? m.identity_users_reset_body()
			: m.identity_users_delete_body()
	}

	get confirmLabel(): string {
		return this.pending?.kind === 'resetTotp'
			? m.identity_users_reset_confirm()
			: m.identity_users_delete_confirm()
	}

	retry = (): void => {
		void this.#users.refetch()
	}

	createInvite = async (): Promise<void> => {
		if (this.#createInvite.isPending) return
		try {
			this.invite = await this.#createInvite.mutateAsync()
		} catch (error) {
			toast.error(m.identity_invite_error(), identityErrorMessage(error))
		} finally {
			// La URL lleva el token en claro: que no quede en el cache de mutaciones.
			this.#createInvite.reset()
		}
	}

	closeInvite = (): void => {
		this.invite = null
	}

	askDelete = (username: string): void => {
		this.pending = { kind: 'delete', username }
	}

	askResetTotp = (username: string): void => {
		this.pending = { kind: 'resetTotp', username }
	}

	cancel = (): void => {
		if (!this.isConfirming) this.pending = null
	}

	confirm = async (): Promise<void> => {
		const pending = this.pending
		if (!pending || this.isConfirming) return
		try {
			if (pending.kind === 'delete') {
				await this.#delete.mutateAsync({
					username: pending.username,
					currentUsername: this.currentUsername,
				})
				toast.success(m.identity_users_deleted({ username: pending.username }))
			} else {
				await this.#resetTotp.mutateAsync(pending.username)
				toast.success(
					m.identity_users_reset_done({ username: pending.username }),
					m.identity_users_reset_done_body(),
				)
			}
		} catch (error) {
			toast.error(identityErrorMessage(error))
		} finally {
			this.pending = null
		}
	}
}
