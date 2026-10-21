import { describe, expect, it, vi } from 'vitest'
import { mountViewModel } from '../../../__tests__/mountViewModel'
import {
	AppConfigLoaderViewModel,
	type loadAppConfigScreen,
} from '../AppConfigLoader.viewModel.svelte.ts'

type ScreenModule = Awaited<ReturnType<typeof loadAppConfigScreen>>

describe('AppConfigLoaderViewModel', () => {
	it('baja la pantalla de Configuración en su propio chunk', async () => {
		const vm = await mountViewModel(() => new AppConfigLoaderViewModel())

		const screen = await vm.screen
		expect(screen.default).toBeTypeOf('function')
	})

	it('si el chunk no baja, Reintentar recarga la página (el import fallido queda guardado)', async () => {
		const load = vi
			.fn<() => Promise<ScreenModule>>()
			.mockRejectedValue(new Error('Failed to fetch dynamically imported module'))
		const reload = vi.fn()
		const vm = await mountViewModel(() => new AppConfigLoaderViewModel(load, reload))

		await expect(vm.screen).rejects.toThrow('Failed to fetch')
		vm.retry()

		expect(reload).toHaveBeenCalledOnce()
		expect(load).toHaveBeenCalledOnce()
	})
})
