import { resolve } from '$app/paths'
import { page } from '$app/state'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import type { SubNavItem } from '@/shared/ui'

// La nav de Ajustes: cada sección es su propia vista (`/settings/proxy`, `/settings/security`,
// …), y la activa es la de la ruta en la que se está.
export class SettingsLayoutViewModel {
	get items(): SubNavItem[] {
		const path = page.url.pathname
		const item = (href: string, label: string): SubNavItem => ({
			href,
			label,
			active: path === href || path.startsWith(`${href}/`),
		})
		return [
			item(resolve('/(app)/settings/proxy'), m.admin_nav_proxy()),
			item(resolve('/(app)/settings/security'), m.admin_nav_security()),
			item(resolve('/(app)/settings/registries'), m.admin_nav_registries()),
			item(resolve('/(app)/settings/users'), m.admin_nav_users()),
			item(resolve('/(app)/settings/tokens'), m.nav_settings_tokens()),
			item(resolve('/(app)/settings/audit'), m.admin_nav_audit()),
		]
	}
}
