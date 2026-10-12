import { createQuery } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { UnavailableError } from '@/shared/domain'
import { settingsQuery } from '../../../data'
import { PROXY_SETTINGS_PHASE, type Settings } from '../../../domain'

// Ajustes (`Ajustes.dc.html`): carga los settings una vez para "Proxy y DNS" y
// "Seguridad". Registries, Usuarios y Auditoría tienen su propia query, y se ven aunque
// los settings fallen.
export class SettingsViewModel {
	readonly proxyPhase = PROXY_SETTINGS_PHASE

	readonly #settings = createQuery(() => settingsQuery())

	get isLoading(): boolean {
		return this.#settings.isPending
	}

	get loadError(): string | null {
		if (!this.#settings.isError) return null
		return this.#settings.error instanceof UnavailableError
			? m.common_unavailable()
			: m.admin_settings_load_error()
	}

	get settings(): Settings | null {
		return this.#settings.data ?? null
	}

	retry = (): void => {
		void this.#settings.refetch()
	}
}
