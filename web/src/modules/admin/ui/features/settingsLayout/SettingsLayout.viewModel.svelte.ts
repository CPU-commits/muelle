import { resolve } from '$app/paths'
import { page } from '$app/state'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { SectionSpy, type SubNavItem } from '@/shared/ui'

/** Las anclas de las secciones de `/ajustes`, en el orden del diseño. */
export const SETTINGS_SECTIONS = {
	proxy: 'proxy',
	security: 'seguridad',
	registries: 'registries',
	users: 'usuarios',
	audit: 'auditoria',
} as const

const onTokens = () => page.url.pathname.startsWith(resolve('/ajustes/tokens'))

// La nav de Ajustes: anclas a las secciones de `/ajustes` y un link a `/ajustes/tokens`. En
// `/ajustes` la activa es la sección que se está leyendo, la del link que se acaba de seguir
// o la del ancla con la que se abrió la página; arriba de todo, la primera (como en el
// diseño). En Tokens, solo Tokens.
export class SettingsLayoutViewModel {
	/** La sección de `/ajustes` que se está leyendo, para la subnav (`<SubNavLayout spy>`). */
	readonly sections = new SectionSpy(Object.values(SETTINGS_SECTIONS), {
		initial: onTokens() ? null : page.url.hash.slice(1),
	})

	get items(): SubNavItem[] {
		const settings = resolve('/ajustes')
		const tokens = resolve('/ajustes/tokens')
		const tokensPage = onTokens()
		const active = tokensPage ? null : (this.sections.current ?? SETTINGS_SECTIONS.proxy)
		const section = (id: string, label: string): SubNavItem => ({
			href: `${settings}#${id}`,
			label,
			active: active === id,
			onselect: () => this.sections.select(id),
		})
		return [
			section(SETTINGS_SECTIONS.proxy, m.admin_nav_proxy()),
			section(SETTINGS_SECTIONS.security, m.admin_nav_security()),
			section(SETTINGS_SECTIONS.registries, m.admin_nav_registries()),
			section(SETTINGS_SECTIONS.users, m.admin_nav_users()),
			{
				href: tokens,
				label: m.nav_settings_tokens(),
				active: tokensPage,
				onselect: () => this.sections.select(null),
			},
			section(SETTINGS_SECTIONS.audit, m.admin_nav_audit()),
		]
	}
}
