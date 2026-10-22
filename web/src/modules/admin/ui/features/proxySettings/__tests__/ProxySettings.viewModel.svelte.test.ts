import { onlineManager, QueryClient } from '@tanstack/svelte-query'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { http, releaseWrites } from '@/shared/data/http/client'
import { createQueryClient } from '@/shared/data/query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { PanelMovedError, UnavailableError, ValidationError } from '@/shared/domain'
import { PORTS_WATCH_TIMEOUT_MS, type ProxySettingsViewModel } from '..'
import { toast } from '@/shared/ui/stores'
import {
	CloudflareUnavailable,
	InvalidIpAddress,
	PortsDockerUnavailable,
	type ProxySettings,
	type Settings,
} from '../../../../domain'
import { ingressKeys, type PortsRollback, type ProxyStatus } from '@/modules/ingress'
import { adminKeys } from '../../../../data'
import ProxySettingsHarness from './ProxySettingsHarness.svelte'
import { cacheHolds } from '../../../__tests__/cacheHolds'

/** Cada cuánto vuelve a pedir `GET /proxy` mientras sigue una recreación (ingress). */
const PROXY_REFRESH_MS = 10_000

const mocks = vi.hoisted(() => ({
	updatePublicIp: vi.fn(),
	updateAcmeEmail: vi.fn(),
	updateProxyPorts: vi.fn(),
	replaceCloudflareToken: vi.fn(),
	removeCloudflareToken: vi.fn(),
}))

// `GET /proxy` (de ingress, por su barrel): Ajustes lo sigue mientras se recrea muelle-caddy.
const ingress = vi.hoisted(() => ({ getProxyStatus: vi.fn() }))
vi.mock('@/modules/ingress/container', () => ({ ingressUseCases: ingress }))

// El origen de la página: el de vitest, salvo que un test entre "por la dirección de Caddy".
const page = vi.hoisted(() => ({ origin: null as string | null }))
vi.mock('../currentOrigin', () => ({
	currentOrigin: () => page.origin ?? location.origin,
}))

function proxyStatus(overrides: Partial<ProxyStatus> = {}): ProxyStatus {
	return {
		status: 'synced',
		error: null,
		syncedAt: new Date('2026-10-16T12:00:00Z'),
		caddy: {
			image: 'ghcr.io/cpu-commits/muelle-caddy:v0.1.0',
			running: true,
			httpPort: 8080,
			httpsPort: 443,
		},
		portsRollback: null,
		...overrides,
	}
}

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
	cloudflare: { tokenSet: false, zones: null, originCa: false },
	publicIpv4: '203.0.113.24',
	publicIpv6: '',
	acmeEmail: 'ops@tudominio.cl',
	panelDomain: 'panel.tudominio.cl',
	httpPort: 80,
	httpsPort: 443,
}

function saved(proxy: Partial<ProxySettings>): Settings {
	return {
		require2fa: false,
		proxy: { ...PROXY, ...proxy },
		panelAccess: { allowedCidrs: [], clientIp: '198.51.100.17' },
	}
}

let client: QueryClient

interface Mounted {
	readonly vm: ProxySettingsViewModel
	/** Salir de Ajustes (navegar a otra pantalla): el viewModel se va, la página sigue. */
	readonly unmount: () => void
}

/** Monta Ajustes; con `queryClient`, el de la app que ya estaba (volver a Ajustes). */
async function mount(
	proxy: ProxySettings = PROXY,
	queryClient: QueryClient = createQueryClient({ retry: false }),
): Promise<Mounted> {
	client = queryClient
	let vm: ProxySettingsViewModel | undefined
	const screen = await render(ProxySettingsHarness, {
		client,
		proxy: () => proxy,
		onReady: (instance) => (vm = instance),
	})
	if (!vm) throw new Error('el harness no expuso el viewModel')
	return { vm, unmount: () => screen.unmount() }
}

async function mountViewModel(proxy: ProxySettings = PROXY): Promise<ProxySettingsViewModel> {
	return (await mount(proxy)).vm
}

describe('ProxySettingsViewModel', () => {
	afterEach(() => releaseWrites())

	beforeEach(() => {
		vi.clearAllMocks()
		vi.restoreAllMocks()
		localStorage.clear()
		page.origin = null
		ingress.getProxyStatus.mockResolvedValue(proxyStatus())
		mocks.updatePublicIp.mockResolvedValue(saved({ publicIpv4: '203.0.113.30' }))
		mocks.updateAcmeEmail.mockResolvedValue(saved({}))
		mocks.updateProxyPorts.mockResolvedValue(saved({ httpPort: 8080, httpsPort: 8443 }))
		mocks.replaceCloudflareToken.mockResolvedValue(
			saved({ cloudflare: { tokenSet: true, zones: 2, originCa: false } }),
		)
		mocks.removeCloudflareToken.mockResolvedValue(
			saved({ cloudflare: { tokenSet: false, zones: null, originCa: false } }),
		)
	})

	it('dice cuántas zonas ve el token, en singular o plural, o solo "cifrado"', async () => {
		const zones = async (count: number | null) =>
			(
				await mountViewModel({
					...PROXY,
					cloudflare: { tokenSet: true, zones: count, originCa: false },
				})
			).cloudflareMeta

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
		const vm = await mountViewModel({
			...PROXY,
			cloudflare: { tokenSet: true, zones: 4, originCa: false },
		})
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

	it('Docker caído al probar los puertos (503): la confirmación queda abierta y dice que no se guardó', async () => {
		const vm = await mountViewModel()
		vm.setField('httpsPort', '8443')
		vm.askSavePorts()

		mocks.updateProxyPorts.mockRejectedValueOnce(new PortsDockerUnavailable())
		await vm.savePorts()
		expect(vm.confirmingPorts).toBe(true)
		expect(vm.portsError).toBe(m.admin_ports_docker_unavailable())
		// No se guardó: ni aviso de mudanza ni seguimiento de muelle-caddy.
		expect(vm.movedTo).toBeNull()
		expect(vm.recreation).toBeNull()
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
		const vm = await mountViewModel({
			...PROXY,
			cloudflare: { tokenSet: true, zones: 4, originCa: false },
		})

		vm.askRemoveCloudflare()
		expect(vm.confirmingRemoveCloudflare).toBe(true)
		expect(mocks.removeCloudflareToken).not.toHaveBeenCalled()

		await vm.removeCloudflare()

		// Con lo que se ve del token: con los certificados de Cloudflare, los apaga también.
		expect(mocks.removeCloudflareToken).toHaveBeenCalledExactlyOnceWith({
			tokenSet: true,
			zones: 4,
			originCa: false,
		})
		expect(vm.confirmingRemoveCloudflare).toBe(false)
		expect(success).toHaveBeenCalledWith(m.admin_cf_removed())
	})

	it('si quitar el token falla, el error queda en la confirmación', async () => {
		mocks.removeCloudflareToken.mockRejectedValueOnce(new UnavailableError('caído'))
		const vm = await mountViewModel({
			...PROXY,
			cloudflare: { tokenSet: true, zones: 4, originCa: false },
		})

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

	it('la confirmación avisa a dónde se muda el panel si cambia el HTTPS, y qué puerto abrir', async () => {
		const vm = await mountViewModel()
		expect(vm.portsMove).toBeNull()

		vm.setField('httpsPort', '8443')
		vm.askSavePorts()
		expect(vm.portsMove).toEqual({ origin: 'https://panel.tudominio.cl:8443', port: 8443 })

		// Solo el HTTP: el panel no se muda.
		vm.cancelPorts()
		vm.setField('httpsPort', '443')
		vm.setField('httpPort', '8080')
		vm.askSavePorts()
		expect(vm.portsMove).toBeNull()

		// Sin dominio del panel, Caddy no lo sirve: tampoco se muda.
		const local = await mountViewModel({ ...PROXY, panelDomain: '' })
		local.setField('httpsPort', '8443')
		local.askSavePorts()
		expect(local.portsMove).toBeNull()
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

	it('el aviso de la dirección nueva sobrevive a recargar hasta que se cierra', async () => {
		const vm = await mountViewModel()
		vm.setField('httpsPort', '8443')
		mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpsPort: 8443 }))
		vm.askSavePorts()
		await vm.savePorts()

		// Recargar es montar otro viewModel: el aviso sigue.
		const reloaded = await mountViewModel({ ...PROXY, httpsPort: 8443 })
		expect(reloaded.movedTo).toBe('https://panel.tudominio.cl:8443')

		reloaded.dismissMove()
		expect(reloaded.movedTo).toBeNull()
		expect((await mountViewModel()).movedTo).toBeNull()
	})

	it('entrando por otra dirección (otro proxy, una IP), el aviso no nombra la de Caddy como la anterior', async () => {
		// Este panel no está en https://panel.tudominio.cl (vitest): esa dirección nunca la usó.
		const vm = await mountViewModel()
		vm.setField('httpsPort', '8443')
		mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpsPort: 8443 }))
		vm.askSavePorts()
		await vm.savePorts()

		expect(vm.movedTo).toBe('https://panel.tudominio.cl:8443')
		expect(vm.movedFrom).toBeNull()
		expect(vm.movedBody).toBe(m.admin_ports_moved_body())
		// Tampoco al recargar.
		expect((await mountViewModel({ ...PROXY, httpsPort: 8443 })).movedFrom).toBeNull()
	})

	it('el aviso de la dirección nueva nombra la anterior: a dónde volver si la nueva no responde', async () => {
		page.origin = 'https://panel.tudominio.cl'
		const vm = await mountViewModel()
		expect(vm.movedBody).toBe(m.admin_ports_moved_body())
		vm.setField('httpsPort', '8443')
		mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpsPort: 8443 }))
		vm.askSavePorts()
		await vm.savePorts()

		const previous = 'https://panel.tudominio.cl'
		expect(vm.movedFrom).toBe(previous)
		expect(vm.movedBody).toBe(m.admin_ports_moved_body_previous({ previous }))
		expect(m.admin_ports_moved_body_previous({ previous })).toContain(previous)

		// Al recargar también; cerrar el aviso la olvida.
		const reloaded = await mountViewModel({ ...PROXY, httpsPort: 8443 })
		expect(reloaded.movedFrom).toBe(previous)
		reloaded.dismissMove()
		expect(reloaded.movedFrom).toBeNull()
		expect(localStorage.getItem('muelle:admin:panel-moved-from')).toBeNull()
	})

	it('una dirección anterior guardada que no es un origen https se ignora', async () => {
		localStorage.setItem('muelle:admin:panel-moved', 'https://panel.tudominio.cl:8443')
		localStorage.setItem('muelle:admin:panel-moved-from', 'javascript:alert(1)')
		const vm = await mountViewModel({ ...PROXY, httpsPort: 8443 })
		expect(vm.movedTo).toBe('https://panel.tudominio.cl:8443')
		expect(vm.movedFrom).toBeNull()
		expect(vm.movedBody).toBe(m.admin_ports_moved_body())
	})

	it('el aviso lleva al panel nuevo con el aviso', async () => {
		const vm = await mountViewModel()
		vm.setField('httpsPort', '8443')
		mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpsPort: 8443 }))
		vm.askSavePorts()
		await vm.savePorts()

		// El link lleva a Ajustes del origen nuevo, que muestra el mismo aviso (la otra
		// pestaña no ve este localStorage: es de otro origen).
		expect(vm.canGoToMovedPanel).toBe(true)
		expect(vm.movedPanelHref).toBe('https://panel.tudominio.cl:8443/ajustes?panel_moved=1')
	})

	it('si se entró por la dirección de Caddy y cambió el HTTPS, no salen más escrituras desde acá', async () => {
		// Esa dirección ya no pasa el chequeo de Origin (contrato §0.1 y §3.5).
		page.origin = 'https://panel.tudominio.cl'
		const vm = await mountViewModel()
		vm.setField('httpsPort', '8443')
		mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpsPort: 8443 }))
		vm.askSavePorts()
		await vm.savePorts()

		const sent = vi.spyOn(globalThis, 'fetch')
		const error = await http.post('/projects').catch((caught: unknown) => caught)
		expect(error).toBeInstanceOf(PanelMovedError)
		expect((error as PanelMovedError).origin).toBe('https://panel.tudominio.cl:8443')
		expect(sent).not.toHaveBeenCalled()
	})

	it('si se entró por otra dirección (otro proxy, desarrollo), las escrituras siguen', async () => {
		const vm = await mountViewModel()
		vm.setField('httpsPort', '8443')
		mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpsPort: 8443 }))
		vm.askSavePorts()
		await vm.savePorts()

		const sent = vi
			.spyOn(globalThis, 'fetch')
			.mockResolvedValue(new Response(null, { status: 204 }))
		await expect(http.post('/projects')).resolves.toBeNull()
		expect(sent).toHaveBeenCalledOnce()
	})

	it('un aviso guardado que no es un origen https se ignora', async () => {
		localStorage.setItem('muelle:admin:panel-moved', 'javascript:alert(1)')
		expect((await mountViewModel()).movedTo).toBeNull()
		localStorage.setItem('muelle:admin:panel-moved', 'https://panel.tudominio.cl/x')
		expect((await mountViewModel()).movedTo).toBeNull()
		// El URL del navegador lo acepta como origen, pero el host no es un dominio ni una IP.
		localStorage.setItem('muelle:admin:panel-moved', 'https://a$(id).com')
		expect((await mountViewModel()).movedTo).toBeNull()
	})

	it('si el panel sigue en este origen, sigue a muelle-caddy (GET /proxy) mientras se recrea', async () => {
		ingress.getProxyStatus.mockResolvedValue(proxyStatus({ status: 'recreating' }))
		const vm = await mountViewModel()
		// Al abrir Ajustes ya se pregunta una vez (por si muelle volvió a los puertos
		// anteriores), pero sin un cambio de puertos acá no hay nada que seguir.
		await vi.waitFor(() => expect(ingress.getProxyStatus).toHaveBeenCalled())
		expect(vm.recreation).toBeNull()

		vm.setField('httpPort', '8080')
		mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpPort: 8080 }))
		vm.askSavePorts()
		await vm.savePorts()

		expect(vm.recreation).toEqual({ kind: 'recreating' })
		await vi.waitFor(() => expect(ingress.getProxyStatus).toHaveBeenCalled())
		expect(vm.recreation).toEqual({ kind: 'recreating' })

		// Ya escucha en los puertos nuevos.
		ingress.getProxyStatus.mockResolvedValue(proxyStatus())
		await client.refetchQueries({ queryKey: ingressKeys.proxy })
		await vi.waitFor(() => expect(vm.recreation).toEqual({ kind: 'done' }))
	})

	it('si otro cambio de puertos falla, no guardó nada: se sigue el anterior', async () => {
		ingress.getProxyStatus.mockResolvedValue(proxyStatus({ status: 'recreating' }))
		const vm = await mountViewModel()
		vm.setField('httpPort', '8080')
		mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpPort: 8080 }))
		vm.askSavePorts()
		await vm.savePorts()
		expect(vm.recreation).toEqual({ kind: 'recreating' })

		vm.setField('httpPort', '8088')
		mocks.updateProxyPorts.mockRejectedValueOnce(new UnavailableError('caído'))
		vm.askSavePorts()
		await vm.savePorts()
		expect(vm.portsError).toBe(m.common_unavailable())
		expect(vm.recreation).toEqual({ kind: 'recreating' })

		ingress.getProxyStatus.mockResolvedValue(proxyStatus())
		await client.refetchQueries({ queryKey: ingressKeys.proxy })
		await vi.waitFor(() => expect(vm.recreation).toEqual({ kind: 'done' }))
	})

	it('si GET /proxy falla mientras muelle-caddy se recrea, sigue diciendo que se recrea', async () => {
		ingress.getProxyStatus.mockResolvedValue(proxyStatus({ status: 'recreating' }))
		const vm = await mountViewModel()
		vm.setField('httpPort', '8080')
		mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpPort: 8080 }))
		vm.askSavePorts()
		await vm.savePorts()
		await vi.waitFor(() => expect(ingress.getProxyStatus).toHaveBeenCalled())

		// El panel lo sirve el mismo muelle-caddy que se está recreando: la petición no llega.
		ingress.getProxyStatus.mockRejectedValue(new UnavailableError('sin red'))
		await client.refetchQueries({ queryKey: ingressKeys.proxy })
		expect(vm.recreation).toEqual({ kind: 'recreating' })

		ingress.getProxyStatus.mockResolvedValue(proxyStatus())
		await client.refetchQueries({ queryKey: ingressKeys.proxy })
		await vi.waitFor(() => expect(vm.recreation).toEqual({ kind: 'done' }))
	})

	it('si GET /proxy falla desde el primer pedido, también: se sigue preguntando', async () => {
		ingress.getProxyStatus.mockRejectedValue(new UnavailableError('sin red'))
		const vm = await mountViewModel()
		// Lo que dejó Dominios antes del cambio: no dice nada de la recreación.
		client.setQueryData(ingressKeys.proxy, proxyStatus(), { updatedAt: Date.now() - 60_000 })
		vm.setField('httpPort', '8080')
		mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpPort: 8080 }))
		vm.askSavePorts()
		await vm.savePorts()

		await vi.waitFor(() => expect(ingress.getProxyStatus).toHaveBeenCalled())
		await vi.waitFor(() =>
			expect(client.getQueryState(ingressKeys.proxy)?.status).toBe('error'),
		)
		expect(vm.recreation).toEqual({ kind: 'recreating' })
	})

	it('pasado el plazo sin una respuesta que diga cómo terminó, dice que no se pudo confirmar', async () => {
		ingress.getProxyStatus.mockRejectedValue(new UnavailableError('sin red'))
		const vm = await mountViewModel()
		vi.useFakeTimers({
			toFake: ['setTimeout', 'clearTimeout', 'Date'],
			shouldAdvanceTime: true,
		})
		try {
			vm.setField('httpPort', '8080')
			mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpPort: 8080 }))
			vm.askSavePorts()
			await vm.savePorts()
			await vi.waitFor(() => expect(ingress.getProxyStatus).toHaveBeenCalled())
			expect(vm.recreation).toEqual({ kind: 'recreating' })

			await vi.advanceTimersByTimeAsync(PORTS_WATCH_TIMEOUT_MS)
			// El aviso no se va sin decir nada: no se sabe si quedó escuchando.
			await vi.waitFor(() => expect(vm.recreation).toEqual({ kind: 'unknown' }))

			// Una respuesta más tarde (recargar, volver a la pestaña) dice cómo terminó.
			ingress.getProxyStatus.mockResolvedValue(proxyStatus())
			await client.refetchQueries({ queryKey: ingressKeys.proxy })
			await vi.waitFor(() => expect(vm.recreation).toEqual({ kind: 'done' }))
		} finally {
			vi.useRealTimers()
		}
	})

	it('pasado el plazo, si GET /proxy sigue fallando deja de preguntar, aunque lo último que dijo fuera "recreando"', async () => {
		const vm = await mountViewModel()
		await vi.waitFor(() => expect(ingress.getProxyStatus).toHaveBeenCalled())
		vi.useFakeTimers({
			toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval', 'Date'],
			shouldAdvanceTime: true,
		})
		try {
			ingress.getProxyStatus.mockResolvedValue(proxyStatus({ status: 'recreating' }))
			vm.setField('httpPort', '8080')
			mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpPort: 8080 }))
			vm.askSavePorts()
			await vm.savePorts()
			// Una respuesta de después del cambio: se está recreando.
			await vi.waitFor(() =>
				expect(client.getQueryState(ingressKeys.proxy)?.status).toBe('success'),
			)
			// Después el panel deja de llegar a muelle: TanStack guarda esa respuesta.
			ingress.getProxyStatus.mockRejectedValue(new UnavailableError('sin red'))
			await vi.advanceTimersByTimeAsync(PORTS_WATCH_TIMEOUT_MS + 2 * PROXY_REFRESH_MS)
			expect(client.getQueryState(ingressKeys.proxy)?.status).toBe('error')

			// Lo último que dijo es "recreando", pero ya no se sabe.
			expect(vm.recreation).toEqual({ kind: 'unknown' })

			// Los 2 min ya pasaron (web/CLAUDE.md): no pregunta más.
			const asked = ingress.getProxyStatus.mock.calls.length
			await vi.advanceTimersByTimeAsync(5 * PROXY_REFRESH_MS)
			expect(ingress.getProxyStatus.mock.calls.length).toBe(asked)
		} finally {
			vi.useRealTimers()
		}
	})

	it('un estado del proxy de antes del cambio (en el cache, de Dominios) no cuenta como "ya escucha"', async () => {
		ingress.getProxyStatus.mockReturnValue(new Promise<ProxyStatus>(() => {}))
		const vm = await mountViewModel()
		// Lo que dejó Dominios hace un minuto.
		client.setQueryData(ingressKeys.proxy, proxyStatus(), { updatedAt: Date.now() - 60_000 })

		vm.setField('httpPort', '8080')
		mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpPort: 8080 }))
		vm.askSavePorts()
		await vm.savePorts()

		await vi.waitFor(() => expect(ingress.getProxyStatus).toHaveBeenCalled())
		expect(vm.recreation).toEqual({ kind: 'recreating' })
	})

	it('si muelle-caddy no arranca con los puertos nuevos, dice por qué (el error de GET /proxy)', async () => {
		ingress.getProxyStatus.mockResolvedValue(
			proxyStatus({
				status: 'out_of_sync',
				error: 'el puerto 8080 ya está en uso',
				caddy: { image: null, running: false, httpPort: null, httpsPort: null },
			}),
		)
		const vm = await mountViewModel()
		vm.setField('httpPort', '8080')
		mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpPort: 8080 }))
		vm.askSavePorts()
		await vm.savePorts()

		await vi.waitFor(() =>
			expect(vm.recreation).toEqual({
				kind: 'failed',
				error: 'el puerto 8080 ya está en uso',
			}),
		)
	})

	it('sin el proxy de muelle (sin dominio del panel) no hay nada que seguir', async () => {
		ingress.getProxyStatus.mockResolvedValue(
			proxyStatus({
				status: 'disabled',
				caddy: { image: null, running: false, httpPort: null, httpsPort: null },
			}),
		)
		const vm = await mountViewModel({ ...PROXY, panelDomain: '' })
		vm.setField('httpPort', '8080')
		mocks.updateProxyPorts.mockResolvedValueOnce(saved({ panelDomain: '', httpPort: 8080 }))
		vm.askSavePorts()
		await vm.savePorts()

		await vi.waitFor(() => expect(ingress.getProxyStatus).toHaveBeenCalled())
		await vi.waitFor(() => expect(vm.recreation).toBeNull())
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

	describe('si muelle-caddy no arranca con los puertos nuevos y muelle vuelve a los anteriores', () => {
		const ROLLBACK: PortsRollback = {
			attempted: { httpPort: 80, httpsPort: 8443 },
			restored: { httpPort: 80, httpsPort: 443 },
			error: 'los puertos 80 o 8443 del host ya están en uso, y muelle-caddy no pudo arrancar con ellos',
			at: new Date('2026-10-16T20:41:07.120Z'),
		}

		async function saveHttps(vm: ProxySettingsViewModel): Promise<void> {
			vm.setField('httpsPort', '8443')
			mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpsPort: 8443 }))
			vm.askSavePorts()
			await vm.savePorts()
		}

		it('al abrir Ajustes por la dirección de Caddy dice que volvió, por qué, y que el panel sigue ahí', async () => {
			page.origin = 'https://panel.tudominio.cl'
			ingress.getProxyStatus.mockResolvedValue(proxyStatus({ portsRollback: ROLLBACK }))
			const vm = await mountViewModel()

			await vi.waitFor(() =>
				expect(vm.rollback).toEqual({
					attempted: { httpPort: 80, httpsPort: 8443 },
					restored: { httpPort: 80, httpsPort: 443 },
					error: ROLLBACK.error,
					at: ROLLBACK.at,
					origin: 'https://panel.tudominio.cl',
				}),
			)
			expect(vm.rollbackBody).toBe(
				`${m.admin_ports_rollback_reason({ http: 80, https: 8443, error: ROLLBACK.error })} ${m.admin_ports_rollback_origin({ origin: 'https://panel.tudominio.cl' })}`,
			)
			// Ya está en esa dirección: no hay a dónde ir.
			expect(vm.rollbackHref).toBeNull()
		})

		it('en la dirección que falló, ofrece volver a la anterior', async () => {
			page.origin = 'https://panel.tudominio.cl:8443'
			ingress.getProxyStatus.mockResolvedValue(proxyStatus({ portsRollback: ROLLBACK }))
			const vm = await mountViewModel({ ...PROXY, httpsPort: 8443 })

			await vi.waitFor(() => expect(vm.rollback?.origin).toBe('https://panel.tudominio.cl'))
			expect(vm.rollbackHref).toBe('https://panel.tudominio.cl/ajustes')
		})

		it('entrando por otra dirección (otro proxy, una IP), no nombra ni ofrece la de Caddy', async () => {
			// vitest no es https://panel.tudominio.cl: esa dirección este panel nunca la usó.
			ingress.getProxyStatus.mockResolvedValue(proxyStatus({ portsRollback: ROLLBACK }))
			const vm = await mountViewModel()

			await vi.waitFor(() => expect(vm.rollback).not.toBeNull())
			expect(vm.rollback?.origin).toBeNull()
			expect(vm.rollbackBody).toBe(
				m.admin_ports_rollback_reason({ http: 80, https: 8443, error: ROLLBACK.error }),
			)
			expect(vm.rollbackHref).toBeNull()
		})

		it('dice cuándo volvió: GET /proxy lo trae hasta el próximo cambio de puertos, quizás días', async () => {
			const at = new Date(Date.now() - 5 * 60_000)
			ingress.getProxyStatus.mockResolvedValue(
				proxyStatus({ portsRollback: { ...ROLLBACK, at } }),
			)
			const vm = await mountViewModel()

			await vi.waitFor(() => expect(vm.rollback?.at).toEqual(at))
			expect(vm.rollbackTitle).toBe(
				'muelle volvió a los puertos anteriores (HTTP 80, HTTPS 443) hace 5 minutos.',
			)
		})

		it('el aviso se cierra: esa vuelta no vuelve a salir (tampoco al recargar), una nueva sí', async () => {
			ingress.getProxyStatus.mockResolvedValue(proxyStatus({ portsRollback: ROLLBACK }))
			const vm = await mountViewModel()
			await vi.waitFor(() => expect(vm.rollback).not.toBeNull())

			vm.dismissRollback()
			expect(vm.rollback).toBeNull()
			expect(vm.rollbackTitle).toBe('')
			expect(vm.rollbackHref).toBeNull()
			expect(vm.recreation).toBeNull()

			// Al recargar, GET /proxy la sigue trayendo: no sale, pero se atiende igual (el aviso
			// guardado de la dirección que falló se va).
			localStorage.setItem('muelle:admin:panel-moved', 'https://panel.tudominio.cl:8443')
			const reloaded = await mountViewModel()
			await vi.waitFor(() => expect(reloaded.movedTo).toBeNull())
			expect(reloaded.rollback).toBeNull()

			// Otra vuelta (otro `at`) sí sale.
			const at = new Date(ROLLBACK.at.getTime() + 60_000)
			ingress.getProxyStatus.mockResolvedValue(
				proxyStatus({ portsRollback: { ...ROLLBACK, at } }),
			)
			const later = await mountViewModel()
			await vi.waitFor(() => expect(later.rollback?.at).toEqual(at))
		})

		it('una vuelta cerrada guardada que no es un momento se ignora', async () => {
			localStorage.setItem('muelle:admin:ports-rollback-dismissed', 'nope')
			ingress.getProxyStatus.mockResolvedValue(proxyStatus({ portsRollback: ROLLBACK }))
			const vm = await mountViewModel()
			await vi.waitFor(() => expect(vm.rollback).not.toBeNull())
		})

		it('después de cambiar el HTTPS: el aviso de la dirección nueva se va y los campos vuelven', async () => {
			const vm = await mountViewModel()
			await vi.waitFor(() => expect(ingress.getProxyStatus).toHaveBeenCalled())
			const invalidate = vi.spyOn(client, 'invalidateQueries')
			await saveHttps(vm)
			expect(vm.movedTo).toBe('https://panel.tudominio.cl:8443')

			ingress.getProxyStatus.mockResolvedValue(proxyStatus({ portsRollback: ROLLBACK }))
			await client.refetchQueries({ queryKey: ingressKeys.proxy })

			await vi.waitFor(() => expect(vm.rollback).not.toBeNull())
			expect(vm.movedTo).toBeNull()
			expect(localStorage.getItem('muelle:admin:panel-moved')).toBeNull()
			expect(vm.recreation).toBeNull()
			expect([vm.httpPort, vm.httpsPort]).toEqual(['80', '443'])
			// Los ajustes de verdad son los de antes: se vuelven a pedir.
			expect(invalidate).toHaveBeenCalledWith({ queryKey: adminKeys.settings() })
		})

		it('entrando por la dirección de Caddy: cuando vuelve a responder, acepta escrituras otra vez', async () => {
			page.origin = 'https://panel.tudominio.cl'
			const vm = await mountViewModel()
			await vi.waitFor(() => expect(ingress.getProxyStatus).toHaveBeenCalled())
			await saveHttps(vm)
			await expect(http.post('/projects')).rejects.toBeInstanceOf(PanelMovedError)

			// Mientras muelle-caddy se recrea, esta dirección no responde: nada que mostrar,
			// el aviso dice a dónde ir.
			ingress.getProxyStatus.mockRejectedValue(new UnavailableError('sin red'))
			await client.refetchQueries({ queryKey: ingressKeys.proxy })
			expect(vm.recreation).toBeNull()
			expect(vm.rollback).toBeNull()

			// Volvió a los puertos anteriores: esta dirección responde de nuevo.
			ingress.getProxyStatus.mockResolvedValue(proxyStatus({ portsRollback: ROLLBACK }))
			await client.refetchQueries({ queryKey: ingressKeys.proxy })
			await vi.waitFor(() => expect(vm.rollback).not.toBeNull())
			expect(vm.movedTo).toBeNull()
			// Ya está acá: no hay a dónde volver.
			expect(vm.rollbackHref).toBeNull()
			const sent = vi
				.spyOn(globalThis, 'fetch')
				.mockResolvedValue(new Response(null, { status: 204 }))
			await expect(http.post('/projects')).resolves.toBeNull()
			expect(sent).toHaveBeenCalledOnce()
		})

		it('entrando por la dirección de Caddy, otro cambio de puertos no sale y no corta el seguimiento', async () => {
			page.origin = 'https://panel.tudominio.cl'
			const vm = await mountViewModel()
			await vi.waitFor(() => expect(ingress.getProxyStatus).toHaveBeenCalled())
			ingress.getProxyStatus.mockRejectedValue(new UnavailableError('sin red'))
			await saveHttps(vm)

			// Se intenta cambiar los puertos otra vez desde acá: el cliente no lo manda.
			vm.setField('httpsPort', '9000')
			const moved = new PanelMovedError('https://panel.tudominio.cl:8443')
			mocks.updateProxyPorts.mockRejectedValueOnce(moved)
			vm.askSavePorts()
			await vm.savePorts()
			expect(vm.portsError).toBe(m.common_panel_moved({ origin: moved.origin }))

			// muelle volvió a los puertos anteriores: el seguimiento lo ve, y queda todo como antes.
			ingress.getProxyStatus.mockResolvedValue(proxyStatus({ portsRollback: ROLLBACK }))
			await client.refetchQueries({ queryKey: ingressKeys.proxy })
			await vi.waitFor(() => expect(vm.rollback).not.toBeNull())
			expect(vm.movedTo).toBeNull()
			expect([vm.httpPort, vm.httpsPort]).toEqual(['80', '443'])
			const sent = vi
				.spyOn(globalThis, 'fetch')
				.mockResolvedValue(new Response(null, { status: 204 }))
			await expect(http.post('/projects')).resolves.toBeNull()
			expect(sent).toHaveBeenCalledOnce()
		})

		it('entrando por la dirección de Caddy y volviendo a Ajustes desde otra pantalla, también acepta escrituras', async () => {
			page.origin = 'https://panel.tudominio.cl'
			const first = await mount()
			await vi.waitFor(() => expect(ingress.getProxyStatus).toHaveBeenCalled())
			// Mientras muelle-caddy se recrea, esta dirección no responde.
			ingress.getProxyStatus.mockRejectedValue(new UnavailableError('sin red'))
			await saveHttps(first.vm)
			await vi.waitFor(() =>
				expect(client.getQueryState(ingressKeys.proxy)?.status).toBe('error'),
			)
			first.unmount()

			// Volvió a los puertos anteriores, y se vuelve a Ajustes (sin recargar: el freno de
			// las escrituras es de la página, no del viewModel que lo puso).
			ingress.getProxyStatus.mockResolvedValue(proxyStatus({ portsRollback: ROLLBACK }))
			const { vm } = await mount({ ...PROXY, httpsPort: 8443 }, client)
			await vi.waitFor(() => expect(vm.rollback).not.toBeNull())
			expect(vm.movedTo).toBeNull()
			const sent = vi
				.spyOn(globalThis, 'fetch')
				.mockResolvedValue(new Response(null, { status: 204 }))
			await expect(http.post('/projects')).resolves.toBeNull()
			expect(sent).toHaveBeenCalledOnce()
		})

		it('con los ajustes del cache en los puertos que fallaron (otra visita a Ajustes), los campos vuelven y se piden de nuevo', async () => {
			const invalidate = vi.spyOn(QueryClient.prototype, 'invalidateQueries')
			ingress.getProxyStatus.mockResolvedValue(proxyStatus({ portsRollback: ROLLBACK }))

			// Los ajustes ya son los de antes: no hay nada que corregir.
			const current = await mountViewModel()
			await vi.waitFor(() => expect(current.rollback).not.toBeNull())
			expect(invalidate).not.toHaveBeenCalledWith({ queryKey: adminKeys.settings() })

			// Se cambió el HTTPS en otra visita, y el cache sigue con el 8443 que falló.
			const vm = await mountViewModel({ ...PROXY, httpsPort: 8443 })
			await vi.waitFor(() => expect(vm.rollback).not.toBeNull())
			expect([vm.httpPort, vm.httpsPort]).toEqual(['80', '443'])
			expect(invalidate).toHaveBeenCalledWith({ queryKey: adminKeys.settings() })
		})

		it('una vuelta atrás de otra dirección no suelta el freno de esta mudanza', async () => {
			// Se cambió el HTTPS a 8443 desde la dirección de Caddy; GET /proxy trae una vuelta
			// atrás que no es de ese puerto (otro intento): las escrituras siguen frenadas.
			page.origin = 'https://panel.tudominio.cl'
			const vm = await mountViewModel()
			await vi.waitFor(() => expect(ingress.getProxyStatus).toHaveBeenCalled())
			await saveHttps(vm)

			ingress.getProxyStatus.mockResolvedValue(
				proxyStatus({
					portsRollback: { ...ROLLBACK, attempted: { httpPort: 80, httpsPort: 9443 } },
				}),
			)
			const { vm: again } = await mount({ ...PROXY, httpsPort: 8443 })
			await vi.waitFor(() => expect(again.rollback).not.toBeNull())
			await expect(http.post('/projects')).rejects.toBeInstanceOf(PanelMovedError)
		})

		it('un ports_rollback de antes del cambio (en el cache) no cuenta: el PATCH lo limpia', async () => {
			ingress.getProxyStatus.mockResolvedValue(proxyStatus({ portsRollback: ROLLBACK }))
			const vm = await mountViewModel()
			await vi.waitFor(() => expect(vm.rollback).not.toBeNull())

			ingress.getProxyStatus.mockReturnValue(new Promise<ProxyStatus>(() => {}))
			vm.setField('httpPort', '8080')
			mocks.updateProxyPorts.mockResolvedValueOnce(saved({ httpPort: 8080 }))
			vm.askSavePorts()
			await vm.savePorts()

			expect(vm.rollback).toBeNull()
			expect(vm.recreation).toEqual({ kind: 'recreating' })
		})

		it('con el aviso guardado de un puerto que falló (al recargar), el aviso se va', async () => {
			localStorage.setItem('muelle:admin:panel-moved', 'https://panel.tudominio.cl:8443')
			ingress.getProxyStatus.mockResolvedValue(proxyStatus({ portsRollback: ROLLBACK }))
			const vm = await mountViewModel()

			await vi.waitFor(() => expect(vm.movedTo).toBeNull())
			expect(localStorage.getItem('muelle:admin:panel-moved')).toBeNull()
		})

		it('si solo cambió el HTTP, no hay otra dirección que ofrecer', async () => {
			ingress.getProxyStatus.mockResolvedValue(
				proxyStatus({
					portsRollback: {
						...ROLLBACK,
						attempted: { httpPort: 9443, httpsPort: 443 },
					},
				}),
			)
			const vm = await mountViewModel()
			await vi.waitFor(() => expect(vm.rollback).not.toBeNull())
			expect(vm.rollback?.origin).toBeNull()
			expect(vm.rollbackHref).toBeNull()
		})
	})
})
