import { resolve } from '$app/paths'
import { page } from '$app/state'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import type { NavItem } from '@/shared/ui'

/** Las anclas de las secciones de `/ajustes`, en el orden del diseño. */
export const SETTINGS_SECTIONS = {
	proxy: 'proxy',
	security: 'seguridad',
	registries: 'registries',
	users: 'usuarios',
	audit: 'auditoria',
} as const

// La nav de Ajustes: anclas a las secciones de `/ajustes` y un link a `/ajustes/tokens`.
// Sin ancla en la URL, la primera sección es la activa (como en el diseño).
export class SettingsLayoutViewModel {
	get items(): NavItem[] {
		const settings = resolve('/ajustes')
		const tokens = resolve('/ajustes/tokens')
		const onTokens = page.url.pathname.startsWith(tokens)
		const hash = onTokens ? null : page.url.hash || `#${SETTINGS_SECTIONS.proxy}`
		const section = (id: string, label: string): NavItem => ({
			href: `${settings}#${id}`,
			label,
			active: hash === `#${id}`,
		})
		return [
			section(SETTINGS_SECTIONS.proxy, m.admin_nav_proxy()),
			section(SETTINGS_SECTIONS.security, m.admin_nav_security()),
			section(SETTINGS_SECTIONS.registries, m.admin_nav_registries()),
			section(SETTINGS_SECTIONS.users, m.admin_nav_users()),
			{ href: tokens, label: m.nav_settings_tokens(), active: onTokens },
			section(SETTINGS_SECTIONS.audit, m.admin_nav_audit()),
		]
	}
}
