import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import {
	band,
	crossing,
	installFakeObserver,
	sectionsPage,
} from '@/shared/ui/components/molecules/subNav/__tests__/fakeIntersectionObserver'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { SETTINGS_SECTIONS, SettingsLayoutViewModel } from '..'

const state = vi.hoisted(() => ({ page: { url: new URL('http://panel.test/ajustes') } }))

// `page` de `$app/state` no existe fuera del runtime de SvelteKit.
vi.mock('$app/state', () => state)

function activeLabel(vm: SettingsLayoutViewModel): string | undefined {
	return vm.items.find((item) => item.active)?.label
}

const SECTIONS = Object.values(SETTINGS_SECTIONS)

describe('SettingsLayoutViewModel', () => {
	beforeEach(() => {
		state.page.url = new URL('http://panel.test/ajustes')
	})

	afterEach(() => {
		vi.unstubAllGlobals()
	})

	it('las secciones son anclas de /ajustes y Tokens es su propia ruta', () => {
		const vm = new SettingsLayoutViewModel()

		expect(vm.items.map((item) => item.href)).toEqual([
			'/ajustes#proxy',
			'/ajustes#seguridad',
			'/ajustes#registries',
			'/ajustes#usuarios',
			'/ajustes/tokens',
			'/ajustes#auditoria',
		])
	})

	it('sin ancla, la primera sección es la activa; con el ancla con la que se abrió, esa', () => {
		expect(activeLabel(new SettingsLayoutViewModel())).toBe(m.admin_nav_proxy())

		state.page.url = new URL('http://panel.test/ajustes#usuarios')
		expect(activeLabel(new SettingsLayoutViewModel())).toBe(m.admin_nav_users())
	})

	it('seguir el link de una sección la marca en el acto; el de Tokens suelta la marca', () => {
		const vm = new SettingsLayoutViewModel()

		vm.items[3]!.onselect?.()
		expect(activeLabel(vm)).toBe(m.admin_nav_users())

		vm.items[4]!.onselect?.()
		expect(activeLabel(vm)).toBe(m.admin_nav_proxy())
	})

	it('la activa es la sección que se está leyendo', () => {
		installFakeObserver()
		const page = sectionsPage(SECTIONS)
		const vm = new SettingsLayoutViewModel()
		const stop = vm.sections.track(page.container)

		band().fire(crossing(SECTIONS, SETTINGS_SECTIONS.registries))
		expect(activeLabel(vm)).toBe(m.admin_nav_registries())

		if (typeof stop === 'function') stop()
		page.remove()
	})

	it('en /ajustes/tokens solo Tokens está activo', () => {
		state.page.url = new URL('http://panel.test/ajustes/tokens#seguridad')
		const vm = new SettingsLayoutViewModel()

		expect(vm.items.filter((item) => item.active).map((item) => item.href)).toEqual([
			'/ajustes/tokens',
		])

		// Aunque haya una sección marcada de antes (el link se siguió y la página cambió).
		vm.items[1]!.onselect?.()
		expect(vm.items.filter((item) => item.active).map((item) => item.href)).toEqual([
			'/ajustes/tokens',
		])
	})
})
