import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SettingsLayoutViewModel } from '..'

const state = vi.hoisted(() => ({ page: { url: new URL('http://panel.test/ajustes') } }))

// `page` de `$app/state` no existe fuera del runtime de SvelteKit.
vi.mock('$app/state', () => state)

function activeLabel(vm: SettingsLayoutViewModel): string | undefined {
	return vm.items.find((item) => item.active)?.label
}

describe('SettingsLayoutViewModel', () => {
	beforeEach(() => {
		state.page.url = new URL('http://panel.test/ajustes')
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

	it('sin ancla, la primera sección es la activa; con ancla, esa', () => {
		const vm = new SettingsLayoutViewModel()
		expect(activeLabel(vm)).toBe(vm.items[0]?.label)

		state.page.url = new URL('http://panel.test/ajustes#usuarios')
		expect(activeLabel(vm)).toBe(vm.items[3]?.label)
	})

	it('en /ajustes/tokens solo Tokens está activo', () => {
		state.page.url = new URL('http://panel.test/ajustes/tokens#seguridad')
		const vm = new SettingsLayoutViewModel()

		expect(vm.items.filter((item) => item.active).map((item) => item.href)).toEqual([
			'/ajustes/tokens',
		])
	})
})
