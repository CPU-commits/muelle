import { afterNavigate, replaceState } from '$app/navigation'
import { resolve } from '$app/paths'
import { createQuery } from '@tanstack/svelte-query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { UnavailableError } from '@/shared/domain'
import { settingsQuery } from '../../../data'
import type { Settings } from '../../../domain'
import { savePanelMove } from '../../features/proxySettings'

/** Las vistas de Ajustes que son de admin, cada una en su ruta (`/settings/<sección>`). */
export type SettingsSection = 'proxy' | 'security' | 'registries' | 'users' | 'audit'

export interface SettingsProps {
	readonly section: SettingsSection
	/**
	 * Se llegó con "Ir al panel" desde la dirección anterior del panel (`?panel_moved=1`):
	 * el aviso de la dirección nueva sigue acá, en el localStorage de este origen.
	 */
	readonly panelMoved?: boolean
}

// "Proxy y DNS" y "Seguridad" leen los settings. Registries, Usuarios y Auditoría tienen su
// propia query: no los piden.
const READS_SETTINGS: ReadonlySet<SettingsSection> = new Set(['proxy', 'security'])

// Una vista de Ajustes (`Ajustes.dc.html`).
export class SettingsViewModel {
	readonly #props: () => SettingsProps
	readonly #settings

	constructor(props: () => SettingsProps) {
		this.#props = props
		this.#settings = createQuery(() => ({
			...settingsQuery(),
			enabled: READS_SETTINGS.has(this.#props().section),
		}))
		if (!props().panelMoved) return
		savePanelMove(location.origin)
		// El parámetro se saca de la URL (una sola vez, con el router andando, como /setup):
		// si quedara, recargar volvería a mostrar un aviso que ya se cerró.
		let dropped = false
		afterNavigate(() => {
			if (dropped) return
			dropped = true
			replaceState(resolve('/(app)/settings/proxy'), {})
		})
	}

	get section(): SettingsSection {
		return this.#props().section
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
