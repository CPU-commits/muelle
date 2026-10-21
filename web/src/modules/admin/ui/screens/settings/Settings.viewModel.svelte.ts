import { afterNavigate, replaceState } from '$app/navigation'
import { resolve } from '$app/paths'
import { createQuery } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { UnavailableError } from '@/shared/domain'
import { settingsQuery } from '../../../data'
import type { Settings } from '../../../domain'
import { savePanelMove } from '../../features/proxySettings'

export interface SettingsProps {
	/**
	 * Se llegó con "Ir al panel" desde la dirección anterior del panel (`?panel_moved=1`):
	 * el aviso de la dirección nueva sigue acá, en el localStorage de este origen.
	 */
	readonly panelMoved?: boolean
}

// Ajustes (`Ajustes.dc.html`): carga los settings una vez para "Proxy y DNS" y
// "Seguridad". Registries, Usuarios y Auditoría tienen su propia query, y se ven aunque
// los settings fallen.
export class SettingsViewModel {
	readonly #settings = createQuery(() => settingsQuery())

	constructor(props: () => SettingsProps = () => ({})) {
		if (!props().panelMoved) return
		savePanelMove(location.origin)
		// El parámetro se saca de la URL (una sola vez, con el router andando, como /setup):
		// si quedara, recargar volvería a mostrar un aviso que ya se cerró.
		let dropped = false
		afterNavigate(() => {
			if (dropped) return
			dropped = true
			replaceState(resolve('/(app)/ajustes'), {})
		})
	}

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
