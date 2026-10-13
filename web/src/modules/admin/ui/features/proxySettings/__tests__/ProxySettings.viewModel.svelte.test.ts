import { QueryClient } from '@tanstack/svelte-query'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { ValidationError } from '@/shared/domain'
import type { ProxySettingsViewModel } from '..'
import { InvalidIpAddress, type ProxySettings, type Settings } from '../../../../domain'
import ProxySettingsHarness from './ProxySettingsHarness.svelte'

const mocks = vi.hoisted(() => ({
	updatePublicIp: vi.fn(),
	updateAcmeEmail: vi.fn(),
	updateProxyPorts: vi.fn(),
	replaceCloudflareToken: vi.fn(),
}))

vi.mock('@/modules/admin/container', () => ({
	adminUseCases: {
		getSettings: vi.fn(),
		updateSecurity: vi.fn(),
		updatePublicIp: mocks.updatePublicIp,
		updateAcmeEmail: mocks.updateAcmeEmail,
		updateProxyPorts: mocks.updateProxyPorts,
		replaceCloudflareToken: mocks.replaceCloudflareToken,
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

/** Si `secret` quedó en algún lado del cache de TanStack (queries o mutaciones). */
async function cacheHolds(secret: string): Promise<boolean> {
	// Una mutación con `gcTime: 0` se borra en el tick siguiente a quedarse sin observers.
	await new Promise((resolve) => setTimeout(resolve, 20))
	const mutations = client
		.getMutationCache()
		.getAll()
		.map((mutation) => mutation.state)
	const queries = client
		.getQueryCache()
		.getAll()
		.map((query) => query.state.data)
	return JSON.stringify([mutations, queries]).includes(secret)
}

async function mountViewModel(): Promise<ProxySettingsViewModel> {
	client = new QueryClient({ defaultOptions: { mutations: { retry: false } } })
	let vm: ProxySettingsViewModel | undefined
	await render(ProxySettingsHarness, {
		client,
		proxy: () => PROXY,
		onReady: (instance) => (vm = instance),
	})
	if (!vm) throw new Error('el harness no expuso el viewModel')
	return vm
}

describe('ProxySettingsViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.updatePublicIp.mockResolvedValue(saved({ publicIpv4: '203.0.113.30' }))
		mocks.updateAcmeEmail.mockResolvedValue(saved({}))
		mocks.updateProxyPorts.mockResolvedValue(saved({ httpPort: 8080, httpsPort: 8443 }))
		mocks.replaceCloudflareToken.mockResolvedValue(
			saved({ cloudflare: { tokenSet: true, zones: 2 } }),
		)
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

	it('el token de Cloudflare se escribe en un modal y no queda en memoria', async () => {
		const vm = await mountViewModel()

		vm.openCloudflare()
		vm.setField('cloudflareToken', 'cf-secreto')
		await vm.saveCloudflare()

		expect(mocks.replaceCloudflareToken).toHaveBeenCalledWith('cf-secreto')
		expect(vm.cloudflareOpen).toBe(false)
		expect(vm.cloudflareToken).toBe('')
		expect(await cacheHolds('cf-secreto')).toBe(false)
	})

	it('si guardar el token falla, tampoco queda en el cache ni en el campo', async () => {
		mocks.replaceCloudflareToken.mockRejectedValueOnce(
			new ValidationError('el token no ve ninguna zona', 'cloudflareToken'),
		)
		const vm = await mountViewModel()

		vm.openCloudflare()
		vm.setField('cloudflareToken', 'cf-malo')
		await vm.saveCloudflare()

		expect(vm.errors).toEqual({ cloudflareToken: 'el token no ve ninguna zona' })
		expect(vm.cloudflareOpen).toBe(true)
		expect(vm.cloudflareToken).toBe('')
		expect(await cacheHolds('cf-malo')).toBe(false)
	})
})
