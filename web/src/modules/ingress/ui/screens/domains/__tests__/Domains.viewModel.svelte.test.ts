import { beforeEach, describe, expect, it, vi } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toasts } from '@/shared/ui/stores'
import { UnavailableError, ValidationError } from '@/shared/domain'
import { DomainNotFound, HostnameTaken, type AppDomain } from '../../../../domain'
import {
	APP,
	makeDomain,
	manualPending,
	recordExists,
} from '../../../../domain/repositories/__fakes__/domainFixtures'
import { mountViewModel } from '../../../__tests__/mountViewModel'
import { domainsQuery, PENDING_REFRESH_MS } from '../../../../data'
import { DomainsViewModel } from '../Domains.viewModel.svelte.ts'

const mocks = vi.hoisted(() => ({
	listDomains: vi.fn(),
	addDomain: vi.fn(),
	setProxied: vi.fn(),
	changeTarget: vi.fn(),
	removeDomain: vi.fn(),
	adoptRecord: vi.fn(),
	retryDomain: vi.fn(),
}))

vi.mock('@/modules/ingress/container', () => ({ ingressUseCases: mocks }))

const ACTIVE = makeDomain('api-staging.tudominio.cl')
const EXISTING = recordExists('webhooks-staging.tudominio.cl')
const MANUAL = manualPending('api.staging.otro-dominio.com')

async function mounted(targets?: string[]) {
	const { vm } = await mountViewModel(
		() => new DomainsViewModel(() => ({ ...APP, ...(targets ? { targets } : {}) })),
	)
	await vi.waitFor(() => expect(vm.domains.isSuccess).toBe(true))
	return vm
}

function toastTitles(): string[] {
	return toasts.items.map((item) => item.title)
}

describe('DomainsViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.listDomains.mockResolvedValue([ACTIVE, MANUAL, EXISTING])
	})

	it('lista los dominios y sugiere destinos (los del catálogo primero)', async () => {
		const vm = await mounted(['web:8080'])

		expect(vm.list).toHaveLength(3)
		expect(vm.suggestions).toEqual(['web:8080', 'api:3000'])
		expect(vm.targetValue).toBe('web:8080')
		expect(vm.targetPlaceholder).toBe(m.ingress_target_placeholder({ service: 'api' }))
	})

	it('valida el formulario antes de llamar al backend', async () => {
		const vm = await mounted()
		vm.setHostname('localhost')
		vm.setTarget('api')

		await vm.add()

		expect(vm.hostnameError).not.toBeNull()
		expect(vm.targetError).not.toBeNull()
		expect(mocks.addDomain).not.toHaveBeenCalled()
	})

	it('agrega con el destino sugerido y Proxied apagado (el default del spec), avisa y limpia el hostname', async () => {
		const created = makeDomain('nuevo.tudominio.cl', { status: 'pending' })
		mocks.addDomain.mockResolvedValue(created)
		const vm = await mounted()
		vm.setHostname('nuevo.tudominio.cl')
		// Después de crear, el backend ya lo lista.
		mocks.listDomains.mockResolvedValue([ACTIVE, MANUAL, EXISTING, created])

		await vm.add()

		expect(mocks.addDomain).toHaveBeenCalledWith({
			app: APP,
			hostname: 'nuevo.tudominio.cl',
			target: 'api:3000',
			proxied: false,
		})
		expect(vm.hostname).toBe('')
		expect(toastTitles()).toContain(m.ingress_toast_added({ hostname: 'nuevo.tudominio.cl' }))
		await vi.waitFor(() =>
			expect(vm.list.some((domain) => domain.hostname === 'nuevo.tudominio.cl')).toBe(true),
		)
	})

	it('Proxied se puede prender en el formulario antes de agregar', async () => {
		mocks.addDomain.mockResolvedValue(makeDomain('nuevo.tudominio.cl', { status: 'pending' }))
		const vm = await mounted()
		expect(vm.proxied).toBe(false)

		vm.setHostname('nuevo.tudominio.cl')
		vm.setProxied(true)
		await vm.add()

		expect(mocks.addDomain).toHaveBeenCalledWith(expect.objectContaining({ proxied: true }))
	})

	it('un hostname tomado o un 422 van al campo que corresponde', async () => {
		const vm = await mounted()
		vm.setHostname('api-staging.tudominio.cl')
		mocks.addDomain.mockRejectedValueOnce(new HostnameTaken())
		await vm.add()
		expect(vm.hostnameError).toBe(m.ingress_error_taken())

		vm.setHostname('otro.tudominio.cl')
		mocks.addDomain.mockRejectedValueOnce(
			new ValidationError('el servicio no existe', 'target'),
		)
		await vm.add()
		expect(vm.targetError).toBe('El servicio no existe')
		expect(vm.hostnameError).toBeNull()
	})

	it('Proxied se mueve enseguida y vuelve si el backend falla', async () => {
		let reject: (error: unknown) => void = () => {}
		mocks.setProxied.mockReturnValue(
			new Promise<AppDomain>((_resolve, rejectFn) => {
				reject = rejectFn
			}),
		)
		const vm = await mounted()

		const pending = vm.toggleProxied(ACTIVE, false)
		await vi.waitFor(() =>
			expect(vm.list.find((domain) => domain.hostname === ACTIVE.hostname)?.proxied).toBe(
				false,
			),
		)
		expect(vm.isBusy(ACTIVE)).toBe(true)

		reject(new UnavailableError('caído'))
		await pending

		expect(vm.list.find((domain) => domain.hostname === ACTIVE.hostname)?.proxied).toBe(true)
		expect(toastTitles()).toContain(m.ingress_toast_error())
	})

	it('adoptar y reintentar llaman al caso de uso de ese dominio', async () => {
		mocks.adoptRecord.mockResolvedValue({ ...EXISTING, recordId: 'nuevo', status: 'pending' })
		mocks.retryDomain.mockResolvedValue({ ...MANUAL, status: 'active' })
		const vm = await mounted()

		await vm.adopt(EXISTING)
		await vm.retry(MANUAL)

		expect(mocks.adoptRecord).toHaveBeenCalledWith({ app: APP, hostname: EXISTING.hostname })
		expect(mocks.retryDomain).toHaveBeenCalledWith({ app: APP, hostname: MANUAL.hostname })
		expect(toastTitles()).toContain(m.ingress_toast_adopted())
	})

	it('editar el destino: valida y guarda', async () => {
		mocks.changeTarget.mockResolvedValue({ ...ACTIVE, target: { service: 'web', port: 8080 } })
		const vm = await mounted()

		vm.openEdit(ACTIVE)
		expect(vm.editTarget).toBe('api:3000')

		vm.setEditTarget('web')
		await vm.saveEdit()
		expect(vm.editError).not.toBeNull()
		expect(mocks.changeTarget).not.toHaveBeenCalled()

		vm.setEditTarget('web:8080')
		await vm.saveEdit()
		expect(mocks.changeTarget).toHaveBeenCalledWith({
			app: APP,
			hostname: ACTIVE.hostname,
			target: 'web:8080',
		})
		expect(vm.editing).toBeNull()
	})

	it('eliminar pide confirmación y lo saca de la lista', async () => {
		mocks.removeDomain.mockResolvedValue(undefined)
		const vm = await mounted()

		vm.askRemove(MANUAL)
		expect(vm.removing?.hostname).toBe(MANUAL.hostname)
		mocks.listDomains.mockResolvedValue([ACTIVE, EXISTING])
		await vm.confirmRemove()

		expect(mocks.removeDomain).toHaveBeenCalledWith({ app: APP, hostname: MANUAL.hostname })
		expect(vm.removing).toBeNull()
		expect(vm.list.some((domain) => domain.hostname === MANUAL.hostname)).toBe(false)
	})

	it('si borrar falla, el error se ve en el diálogo, que queda abierto', async () => {
		mocks.removeDomain.mockRejectedValueOnce(new UnavailableError('caído'))
		const vm = await mounted()

		const toastsBefore = toastTitles().length
		vm.askRemove(MANUAL)
		await vm.confirmRemove()

		expect(vm.removing?.hostname).toBe(MANUAL.hostname)
		expect(vm.removeError).not.toBeNull()
		// Sin toast: el error ya está en el diálogo.
		expect(toastTitles()).toHaveLength(toastsBefore)

		vm.cancelRemove()
		expect(vm.removing).toBeNull()
		expect(vm.removeError).toBeNull()
	})

	it('si otra persona ya lo borró, se cierra y se pide la lista de nuevo', async () => {
		mocks.removeDomain.mockRejectedValueOnce(new DomainNotFound())
		const vm = await mounted()
		mocks.listDomains.mockResolvedValue([ACTIVE, EXISTING])

		vm.askRemove(MANUAL)
		await vm.confirmRemove()

		expect(vm.removing).toBeNull()
		expect(vm.removeError).toBeNull()
		await vi.waitFor(() =>
			expect(vm.list.some((domain) => domain.hostname === MANUAL.hostname)).toBe(false),
		)
	})

	it('los dominios con error se anuncian en una sola región, sin repetir en cada refresco', async () => {
		const broken = makeDomain('roto.tudominio.cl', { status: 'error', errorKind: 'cloudflare' })
		mocks.listDomains.mockResolvedValue([ACTIVE, EXISTING, broken])
		const vm = await mounted()

		const announcement = vm.failingAnnouncement
		expect(announcement).toBe(
			m.ingress_failing_announce({
				count: 2,
				hostnames: 'roto.tudominio.cl, webhooks-staging.tudominio.cl',
			}),
		)
		await vm.domains.refetch()
		expect(vm.failingAnnouncement).toBe(announcement)
	})

	it('se refresca sola mientras muelle provisiona, no por un registro manual', () => {
		const interval = domainsQuery(APP).refetchInterval
		if (typeof interval !== 'function') throw new Error('sin refetchInterval')
		const withData = (data: readonly AppDomain[]) =>
			interval({ state: { data } } as Parameters<typeof interval>[0])

		expect(withData([ACTIVE, MANUAL])).toBe(false)
		expect(withData([ACTIVE, makeDomain('nuevo.tudominio.cl', { status: 'pending' })])).toBe(
			PENDING_REFRESH_MS,
		)
	})

	it('sin dominios queda vacío, con el formulario', async () => {
		mocks.listDomains.mockResolvedValue([])
		const vm = await mounted()
		expect(vm.isEmpty).toBe(true)
		expect(vm.suggestions).toEqual([])
	})

	it('contra un backend sin la Fase 3 muestra la fase', async () => {
		mocks.listDomains.mockRejectedValue(
			new UnavailableError('Llega con la Fase 3', { phase: 3 }),
		)
		const { vm } = await mountViewModel(() => new DomainsViewModel(() => APP))

		await vi.waitFor(() => expect(vm.pendingPhase).toBe(3))
		expect(vm.loadError).toBeNull()
	})

	it('un backend caído es un error con reintento', async () => {
		mocks.listDomains.mockRejectedValue(new UnavailableError('caído'))
		const { vm } = await mountViewModel(() => new DomainsViewModel(() => APP))

		await vi.waitFor(() => expect(vm.loadError).toBeInstanceOf(UnavailableError))
		mocks.listDomains.mockResolvedValue([ACTIVE])
		vm.retryLoad()
		await vi.waitFor(() => expect(vm.list).toHaveLength(1))
	})
})
