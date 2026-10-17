import { onlineManager, type QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { UnavailableError, ValidationError } from '@/shared/domain'
import type { ProxySettingsViewModel } from '..'
import { toast } from '@/shared/ui/stores'
import {
	CloudflareUnavailable,
	InvalidIpAddress,
	type ProxySettings,
	type Settings,
} from '../../../../domain'
import { ingressKeys } from '@/modules/ingress'
import ProxySettingsHarness from './ProxySettingsHarness.svelte'
import { cacheHolds } from '../../../__tests__/cacheHolds'

const mocks = vi.hoisted(() => ({
	updatePublicIp: vi.fn(),
	updateAcmeEmail: vi.fn(),
	updateProxyPorts: vi.fn(),
	replaceCloudflareToken: vi.fn(),
	removeCloudflareToken: vi.fn(),
}))

vi.mock('@/modules/admin/container', () => ({
	adminUseCases: {
		getSettings: vi.fn(),
		updateSecurity: vi.fn(),
		updatePublicIp: mocks.updatePublicIp,
		updateAcmeEmail: mocks.updateAcmeEmail,
		updateProxyPorts: mocks.updateProxyPorts,
		replaceCloudflareToken: mocks.replaceCloudflareToken,
		removeCloudflareToken: mocks.removeCloudflareToken,
		listAudit: vi.fn(),
		listRegistries: vi.fn(),
		addRegistry: vi.fn(),
		updateRegistry: vi.fn(),
		removeRegistry: vi.fn(),
	},
}))

const PROXY: ProxySettings = {
	cloudflare: { tokenSet: false, zones: null },
	publicIpv4: '203.0.113.24',
	publicIpv6: '',
	acmeEmail: 'ops@tudominio.cl',
	panelDomain: 'panel.tudominio.cl',
	httpPort: 80,
	httpsPort: 443,
}

function saved(proxy: Partial<ProxySettings>): Settings {
	return { require2fa: false, proxy: { ...PROXY, ...proxy }, panelAccess: null }
}

let client: QueryClient

async function mountViewModel(proxy: ProxySettings = PROXY): Promise<ProxySettingsViewModel> {
	client = createQueryClient({ retry: false })
	let vm: ProxySettingsViewModel | undefined
	await render(ProxySettingsHarness, {
		client,
		proxy: () => proxy,
		onReady: (instance) => (vm = instance),
	})
	if (!vm) throw new Error('el harness no expuso el viewModel')
	return vm
}

describe('ProxySettingsViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		vi.restoreAllMocks()
		mocks.updatePublicIp.mockResolvedValue(saved({ publicIpv4: '203.0.113.30' }))
		mocks.updateAcmeEmail.mockResolvedValue(saved({}))
		mocks.updateProxyPorts.mockResolvedValue(saved({ httpPort: 8080, httpsPort: 8443 }))
		mocks.replaceCloudflareToken.mockResolvedValue(
			saved({ cloudflare: { tokenSet: true, zones: 2 } }),
		)
		mocks.removeCloudflareToken.mockResolvedValue(
			saved({ cloudflare: { tokenSet: false, zones: null } }),
		)
	})

	it('dice cuántas zonas ve el token, en singular o plural, o solo "cifrado"', async () => {
		const zones = async (count: number | null) =>
			(await mountViewModel({ ...PROXY, cloudflare: { tokenSet: true, zones: count } }))
				.cloudflareMeta

		expect(await zones(4)).toBe(m.admin_cf_zones({ count: 4 }))
		expect(await zones(1)).toBe(m.admin_cf_zones_one())
		expect(await zones(null)).toBe(m.admin_cf_encrypted())
		expect(m.admin_cf_zones({ count: 4 })).toMatch(/^4 zonas visibles/)
	})

	it('cada tarjeta se activa cuando algo suyo cambió', async () => {
		const vm = await mountViewModel()
		expect([vm.isIpDirty, vm.isAcmeDirty, vm.arePortsDirty]).toEqual([false, false, false])

		vm.setField('ipv4', '203.0.113.30')
		expect([vm.isIpDirty, vm.isAcmeDirty, vm.arePortsDirty]).toEqual([true, false, false])
	})

	it('guardar la IP no pisa lo que se está escribiendo en otra tarjeta', async () => {
		const vm = await mountViewModel()
		vm.setField('ipv4', '203.0.113.30')
		vm.setField('acmeEmail', 'certs@tudominio.cl')

		await vm.saveIp()

		expect(mocks.updatePublicIp).toHaveBeenCalledWith({ ipv4: '203.0.113.30', ipv6: '' })
		expect(vm.acmeEmail).toBe('certs@tudominio.cl')
	})

	it('cada cambio que sale bien vuelve a pedir los dominios y el proxy (records, estado)', async () => {
		const vm = await mountViewModel({ ...PROXY, cloudflare: { tokenSet: true, zones: 4 } })
		const invalidate = vi.spyOn(client, 'invalidateQueries')
		const refreshedIngress = () =>
			invalidate.mock.calls.filter(([filters]) => filters?.queryKey === ingressKeys.all)
				.length

		vm.setField('ipv4', '203.0.113.30')
		await vm.saveIp()
		expect(refreshedIngress()).toBe(1)

		vm.setField('acmeEmail', 'certs@tudominio.cl')
		await vm.saveAcme()
		expect(refreshedIngress()).toBe(2)

		vm.setField('httpPort', '8080')
		vm.askSavePorts()
		await vm.savePorts()
		expect(refreshedIngress()).toBe(3)

		vm.openCloudflare()
		vm.setField('cloudflareToken', 'x'.repeat(40))
		await vm.saveCloudflare()
		expect(refreshedIngress()).toBe(4)

		vm.askRemoveCloudflare()
		await vm.removeCloudflare()
		expect(refreshedIngress()).toBe(5)

		// Si falla, no cambió nada: no hay qué volver a pedir.
		mocks.updatePublicIp.mockRejectedValueOnce(new UnavailableError('caído'))
		vm.setField('ipv4', '203.0.113.31')
		await vm.saveIp()
		expect(refreshedIngress()).toBe(5)
	})

	it('los errores van a su campo y se limpian al escribir', async () => {
		const vm = await mountViewModel()
		mocks.updatePublicIp.mockRejectedValueOnce(new InvalidIpAddress(6, 'ipv6'))
		vm.setField('ipv6', 'nope')

		await vm.saveIp()
		expect(vm.errors).toEqual({ ipv6: m.admin_error_ipv6() })

		vm.setField('ipv6', '2001:db8::1')
		expect(vm.errors).toEqual({})

		mocks.updateAcmeEmail.mockRejectedValueOnce(new ValidationError('rebota', 'acmeEmail'))
		vm.setField('acmeEmail', 'x@y.cl')
		await vm.saveAcme()
		expect(vm.errors).toEqual({ acmeEmail: 'rebota' })
	})

	it('los puertos se validan antes de pedir confirmación', async () => {
		const vm = await mountViewModel()

		vm.setField('httpsPort', '80')
		vm.askSavePorts()
		expect(vm.confirmingPorts).toBe(false)
		expect(vm.errors).toEqual({ httpsPort: m.admin_error_port_same() })

		vm.setField('httpPort', '8080')
		vm.setField('httpsPort', '8443')
		vm.askSavePorts()
		expect(vm.confirmingPorts).toBe(true)
		expect(mocks.updateProxyPorts).not.toHaveBeenCalled()

		await vm.savePorts()
		expect(mocks.updateProxyPorts).toHaveBeenCalledWith({ http: '8080', https: '8443' })
		expect(vm.confirmingPorts).toBe(false)
	})

	it('si cambiar los puertos falla, el error queda en la confirmación; uno de campo, en la tarjeta', async () => {
		const vm = await mountViewModel()
		vm.setField('httpPort', '8080')
		vm.askSavePorts()

		mocks.updateProxyPorts.mockRejectedValueOnce(new UnavailableError('caído'))
		await vm.savePorts()
		expect(vm.confirmingPorts).toBe(true)
		expect(vm.portsError).toBe(m.common_unavailable())

		mocks.updateProxyPorts.mockRejectedValueOnce(
			new ValidationError('el puerto está ocupado', 'httpPort'),
		)
		await vm.savePorts()
		expect(vm.confirmingPorts).toBe(false)
		expect(vm.errors).toEqual({ httpPort: 'el puerto está ocupado' })
	})

	it('el token de Cloudflare se escribe en un modal y no queda en memoria', async () => {
		const vm = await mountViewModel()

		vm.openCloudflare()
		vm.setField('cloudflareToken', 'cf-secreto-de-veinte-caracteres')
		await vm.saveCloudflare()

		expect(mocks.replaceCloudflareToken).toHaveBeenCalledWith('cf-secreto-de-veinte-caracteres')
		expect(vm.cloudflareOpen).toBe(false)
		expect(vm.cloudflareToken).toBe('')
		expect(await cacheHolds(client, 'cf-secreto-de-veinte-caracteres')).toBe(false)
	})

	it('sin red, guardar el token falla enseguida en vez de quedar pausado con el token', async () => {
		mocks.replaceCloudflareToken.mockRejectedValueOnce(new UnavailableError('sin red'))
		const vm = await mountViewModel()
		onlineManager.setOnline(false)
		try {
			vm.openCloudflare()
			vm.setField('cloudflareToken', 'cf-sin-red-de-veinte-caracteres')
			// Pausada, esta promesa no terminaría hasta que vuelva la red.
			await vm.saveCloudflare()

			expect(mocks.replaceCloudflareToken).toHaveBeenCalledWith(
				'cf-sin-red-de-veinte-caracteres',
			)
			expect(await cacheHolds(client, 'cf-sin-red-de-veinte-caracteres')).toBe(false)
		} finally {
			onlineManager.setOnline(true)
		}
	})

	it('un token sin la forma de uno de Cloudflare no sale y se queda en el campo para corregirlo', async () => {
		const vm = await mountViewModel()

		vm.openCloudflare()
		vm.setField('cloudflareToken', 'cf-corto')
		await vm.saveCloudflare()

		expect(mocks.replaceCloudflareToken).not.toHaveBeenCalled()
		expect(vm.errors).toEqual({
			cloudflareToken: m.admin_error_cf_token_length({ min: 20, max: 512 }),
		})
		expect(vm.cloudflareToken).toBe('cf-corto')
		expect(vm.cloudflareOpen).toBe(true)

		vm.setField('cloudflareToken', '  ')
		await vm.saveCloudflare()
		expect(vm.errors).toEqual({ cloudflareToken: m.common_required() })
		expect(vm.cloudflareToken).toBe('  ')
		expect(mocks.replaceCloudflareToken).not.toHaveBeenCalled()
	})

	it('si guardar el token falla, tampoco queda en el cache ni en el campo', async () => {
		mocks.replaceCloudflareToken.mockRejectedValueOnce(
			new ValidationError('el token no ve ninguna zona', 'cloudflareToken'),
		)
		const vm = await mountViewModel()

		vm.openCloudflare()
		vm.setField('cloudflareToken', 'cf-malo-de-veinte-caracteres')
		await vm.saveCloudflare()

		expect(vm.errors).toEqual({ cloudflareToken: 'el token no ve ninguna zona' })
		expect(vm.cloudflareOpen).toBe(true)
		expect(vm.cloudflareToken).toBe('')
		expect(await cacheHolds(client, 'cf-malo-de-veinte-caracteres')).toBe(false)
	})

	it('vaciar la IPv4 o el email ACME los borra: se manda ""', async () => {
		const vm = await mountViewModel()

		vm.setField('ipv4', '')
		expect(vm.isIpDirty).toBe(true)
		await vm.saveIp()
		vm.setField('acmeEmail', '  ')
		expect(vm.isAcmeDirty).toBe(true)
		await vm.saveAcme()

		expect(mocks.updatePublicIp).toHaveBeenCalledWith({ ipv4: '', ipv6: '' })
		expect(mocks.updateAcmeEmail).toHaveBeenCalledWith('  ')
	})

	it('quitar el token pide confirmación y lo borra', async () => {
		const success = vi.spyOn(toast, 'success')
		const vm = await mountViewModel({ ...PROXY, cloudflare: { tokenSet: true, zones: 4 } })

		vm.askRemoveCloudflare()
		expect(vm.confirmingRemoveCloudflare).toBe(true)
		expect(mocks.removeCloudflareToken).not.toHaveBeenCalled()

		await vm.removeCloudflare()

		expect(mocks.removeCloudflareToken).toHaveBeenCalledOnce()
		expect(vm.confirmingRemoveCloudflare).toBe(false)
		expect(success).toHaveBeenCalledWith(m.admin_cf_removed())
	})

	it('si quitar el token falla, el error queda en la confirmación', async () => {
		mocks.removeCloudflareToken.mockRejectedValueOnce(new UnavailableError('caído'))
		const vm = await mountViewModel({ ...PROXY, cloudflare: { tokenSet: true, zones: 4 } })

		vm.askRemoveCloudflare()
		await vm.removeCloudflare()

		expect(vm.confirmingRemoveCloudflare).toBe(true)
		expect(vm.removeCloudflareError).toBe(m.common_unavailable())
		vm.cancelRemoveCloudflare()
		expect(vm.confirmingRemoveCloudflare).toBe(false)
		expect(vm.removeCloudflareError).toBeNull()
	})

	it('Cloudflare caído (502) no es un error del campo: un aviso en el modal para reintentar', async () => {
		const failure = vi.spyOn(toast, 'error')
		mocks.replaceCloudflareToken.mockRejectedValueOnce(new CloudflareUnavailable())
		const vm = await mountViewModel()

		vm.openCloudflare()
		vm.setField('cloudflareToken', 'cf-cloudflare-caido-de-veinte')
		await vm.saveCloudflare()

		expect(vm.errors).toEqual({})
		expect(vm.cloudflareError).toEqual({
			title: m.admin_cf_unavailable_title(),
			body: m.admin_cf_unavailable_body(),
		})
		expect(vm.cloudflareOpen).toBe(true)
		expect(failure).not.toHaveBeenCalled()
		// El token tampoco se queda: se pega otra vez para reintentar.
		expect(vm.cloudflareToken).toBe('')
		expect(await cacheHolds(client, 'cf-cloudflare-caido-de-veinte')).toBe(false)

		// Reintentar limpia el aviso; cerrar y volver a abrir, también.
		vm.setField('cloudflareToken', 'cf-otra-vez-de-veinte-caracteres')
		await vm.saveCloudflare()
		expect(vm.cloudflareError).toBeNull()
		expect(vm.cloudflareOpen).toBe(false)
	})

	it('cualquier otro error del modal del token también queda adentro', async () => {
		mocks.replaceCloudflareToken.mockRejectedValueOnce(new UnavailableError('sin red'))
		const vm = await mountViewModel()

		vm.openCloudflare()
		vm.setField('cloudflareToken', 'cf-x-de-veinte-caracteres')
		await vm.saveCloudflare()

		expect(vm.cloudflareError).toEqual({ title: m.common_unavailable(), body: null })
		vm.closeCloudflare()
		vm.openCloudflare()
		expect(vm.cloudflareError).toBeNull()
	})

	it('mientras se aplican los puertos (hasta 2 min) la confirmación dice que está aplicando', async () => {
		let finish: (settings: Settings) => void = () => {}
		mocks.updateProxyPorts.mockReturnValueOnce(
			new Promise<Settings>((resolve) => (finish = resolve)),
		)
		const vm = await mountViewModel()
		vm.setField('httpPort', '8080')
		vm.askSavePorts()

		const saving = vm.savePorts()
		await expect.poll(() => vm.isSavingPorts).toBe(true)
		expect(vm.confirmingPorts).toBe(true)
		// Ni cancelar ni cerrar la cortan: el backend ya está aplicando.
		vm.cancelPorts()
		expect(vm.confirmingPorts).toBe(true)

		finish(saved({ httpPort: 8080 }))
		await saving
		await expect.poll(() => vm.isSavingPorts).toBe(false)
		expect(vm.confirmingPorts).toBe(false)
	})

	it('si cambió el puerto HTTPS, avisa a dónde se muda el panel', async () => {
		const vm = await mountViewModel()
		vm.setField('httpsPort', '8443')
		mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpsPort: 8443 }))
		vm.askSavePorts()
		await vm.savePorts()

		expect(vm.movedTo).toBe('https://panel.tudominio.cl:8443')

		// Volver al 443 deja el origen sin puerto.
		const back = await mountViewModel({ ...PROXY, httpsPort: 8443 })
		back.setField('httpsPort', '443')
		mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpsPort: 443 }))
		back.askSavePorts()
		await back.savePorts()
		expect(back.movedTo).toBe('https://panel.tudominio.cl')
	})

	it('si solo cambió el HTTP, o no hay dominio del panel, no hay mudanza', async () => {
		const vm = await mountViewModel()
		vm.setField('httpPort', '8080')
		mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpPort: 8080 }))
		vm.askSavePorts()
		await vm.savePorts()
		expect(vm.movedTo).toBeNull()

		const local = await mountViewModel({ ...PROXY, panelDomain: '' })
		local.setField('httpsPort', '8443')
		mocks.updateProxyPorts.mockResolvedValueOnce(saved({ panelDomain: '', httpsPort: 8443 }))
		local.askSavePorts()
		await local.savePorts()
		expect(local.movedTo).toBeNull()
	})
})
