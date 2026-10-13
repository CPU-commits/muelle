import { beforeEach, describe, expect, it, vi } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toasts } from '@/shared/ui/stores'
import { UnavailableError, ValidationError } from '@/shared/domain'
import { HostnameTaken, type AppDomain } from '../../../../domain'
import {
	APP,
	makeDomain,
	manualPending,
	recordExists,
} from '../../../../domain/repositories/__fakes__/domainFixtures'
import { mountViewModel } from '../../../__tests__/mountViewModel'
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

	it('agrega con el destino sugerido, avisa y limpia el hostname', async () => {
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
			proxied: true,
		})
		expect(vm.hostname).toBe('')
		expect(toastTitles()).toContain(m.ingress_toast_added({ hostname: 'nuevo.tudominio.cl' }))
		await vi.waitFor(() =>
			expect(vm.list.some((domain) => domain.hostname === 'nuevo.tudominio.cl')).toBe(true),
		)
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
