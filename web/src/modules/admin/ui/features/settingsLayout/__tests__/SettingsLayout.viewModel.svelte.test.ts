import { beforeEach, describe, expect, it, vi } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { SettingsLayoutViewModel } from '..'

const state = vi.hoisted(() => ({ page: { url: new URL('http://panel.test/settings/proxy') } }))

// `page` de `$app/state` no existe fuera del runtime de SvelteKit.
vi.mock('$app/state', () => state)

function activeLabels(vm: SettingsLayoutViewModel): string[] {
	return vm.items.filter((item) => item.active).map((item) => item.label)
}

describe('SettingsLayoutViewModel', () => {
	beforeEach(() => {
		state.page.url = new URL('http://panel.test/settings/proxy')
	})

	it('cada sección es su propia ruta, en el orden del diseño', () => {
		const vm = new SettingsLayoutViewModel()

		expect(vm.items.map((item) => item.href)).toEqual([
			'/settings/proxy',
			'/settings/security',
			'/settings/registries',
			'/settings/users',
			'/settings/tokens',
			'/settings/audit',
		])
	})

	it('la activa es la de la ruta, y solo esa', () => {
		expect(activeLabels(new SettingsLayoutViewModel())).toEqual([m.admin_nav_proxy()])

		state.page.url = new URL('http://panel.test/settings/users')
		expect(activeLabels(new SettingsLayoutViewModel())).toEqual([m.admin_nav_users()])

		state.page.url = new URL('http://panel.test/settings/tokens')
		expect(activeLabels(new SettingsLayoutViewModel())).toEqual([m.nav_settings_tokens()])
	})

	it('una ruta que solo empieza igual no marca la sección', () => {
		state.page.url = new URL('http://panel.test/settings/proxy-viejo')

		expect(activeLabels(new SettingsLayoutViewModel())).toEqual([])
	})
})
