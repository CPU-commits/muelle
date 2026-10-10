import { goto } from '$app/navigation'
import { resolve } from '$app/paths'
import { createMutation, createQuery, useQueryClient } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import type { MenuEntry } from '@/shared/ui'
import { localeLabel, nextLocale, setLocale } from '@/shared/ui/i18n'
import { logoutMutation, meQuery } from '../../../data'
import type { Me } from '../../../domain'

// La sesión dentro del marco de la app: quién es (para el avatar y el menú de la cuenta)
// y cerrar sesión. `initial` es el `me` que cargó el guard de `(app)`.
export class SessionShellViewModel {
	readonly #initial: () => Me
	readonly #client = useQueryClient()
	readonly #me = createQuery(() => meQuery())
	readonly #logout = createMutation(() => logoutMutation(this.#client))

	constructor(initial: () => Me) {
		this.#initial = initial
	}

	get me(): Me {
		return this.#me.data ?? this.#initial()
	}

	get username(): string {
		return this.me.username
	}

	get totpEnabled(): boolean {
		return this.me.totpEnabled
	}

	get isLoggingOut(): boolean {
		return this.#logout.isPending
	}

	get accountItems(): readonly MenuEntry[] {
		const other = nextLocale()
		return [
			{
				kind: 'link',
				id: 'totp',
				label: m.nav_account_2fa(),
				href: resolve('/2fa'),
				icon: 'shield',
			},
			{
				kind: 'action',
				id: 'locale',
				label: localeLabel(other),
				icon: 'globe',
				onSelect: () => void setLocale(other),
			},
			{ kind: 'separator', id: 'separator' },
			{
				kind: 'action',
				id: 'logout',
				label: m.nav_account_logout(),
				icon: 'logOut',
				disabled: this.isLoggingOut,
				onSelect: () => void this.logout(),
			},
		]
	}

	logout = async (): Promise<void> => {
		await this.#logout.mutateAsync()
		await goto(resolve('/login'))
	}
}
