import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import type { SecuritySettingsViewModel } from '..'
import { UnavailableError, ValidationError } from '@/shared/domain'
import { type Settings, TotpRequiredFirst } from '../../../../domain'
import SecuritySettingsHarness from './SecuritySettingsHarness.svelte'

const mocks = vi.hoisted(() => ({ updateSecurity: vi.fn() }))

// Se mockea el CONTAINER, no la red.
vi.mock('@/modules/admin/container', () => ({
	adminUseCases: {
		getSettings: vi.fn(),
		updateSecurity: mocks.updateSecurity,
		updatePublicIp: vi.fn(),
		updateAcmeEmail: vi.fn(),
		updateProxyPorts: vi.fn(),
		replaceCloudflareToken: vi.fn(),
		listAudit: vi.fn(),
		listRegistries: vi.fn(),
		addRegistry: vi.fn(),
		updateRegistry: vi.fn(),
		removeRegistry: vi.fn(),
	},
}))

const PHASE_3: Settings = {
	require2fa: false,
	proxy: null,
	panelAccess: { allowedCidrs: ['198.51.100.0/24'], clientIp: '198.51.100.17' },
}

const PHASE_1: Settings = { require2fa: false, proxy: null, panelAccess: null }

async function mountViewModel(initial: Settings): Promise<SecuritySettingsViewModel> {
	const client = createQueryClient({ retry: false })
	let current = initial
	mocks.updateSecurity.mockImplementation(
		(input: { require2fa?: boolean; allowedCidrs?: string }) => {
			current = {
				...current,
				require2fa: input.require2fa ?? current.require2fa,
				panelAccess: current.panelAccess && {
					...current.panelAccess,
					allowedCidrs:
						input.allowedCidrs === undefined
							? current.panelAccess.allowedCidrs
							: input.allowedCidrs.split('\n').filter(Boolean),
				},
			}
			return Promise.resolve(current)
		},
	)
	let vm: SecuritySettingsViewModel | undefined
	await render(SecuritySettingsHarness, {
		client,
		settings: () => current,
		onReady: (instance) => (vm = instance),
	})
	if (!vm) throw new Error('el harness no expuso el viewModel')
	return vm
}

describe('SecuritySettingsViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		vi.restoreAllMocks()
	})

	it('arranca con lo guardado y sin cambios', async () => {
		const vm = await mountViewModel(PHASE_3)

		expect(vm.require2fa).toBe(false)
		expect(vm.cidrs).toBe('198.51.100.0/24')
		expect(vm.isDirty).toBe(false)
		expect(vm.clientIpHint).toBe(m.admin_security_client_ip_in({ ip: '198.51.100.17' }))
	})

	it('manda solo lo que cambió', async () => {
		const success = vi.spyOn(toast, 'success')
		const vm = await mountViewModel(PHASE_3)

		vm.setRequire2fa(true)
		await vm.save()

		expect(mocks.updateSecurity).toHaveBeenCalledWith({ require2fa: true })
		expect(success).toHaveBeenCalledWith(m.admin_security_saved())
		expect(vm.isDirty).toBe(false)
	})

	it('totp_required_first: vuelve el switch y avisa con el link a /2fa', async () => {
		const vm = await mountViewModel(PHASE_1)
		mocks.updateSecurity.mockRejectedValueOnce(new TotpRequiredFirst())

		vm.setRequire2fa(true)
		await vm.save()

		expect(vm.totpRequiredFirst).toBe(true)
		expect(vm.require2fa).toBe(false)
		// Volver a tocar el switch esconde el aviso.
		vm.setRequire2fa(true)
		expect(vm.totpRequiredFirst).toBe(false)
	})

	it('en la Fase 1 no hay IPs permitidas y no se mandan', async () => {
		const vm = await mountViewModel(PHASE_1)

		vm.setCidrs('10.0.0.0/8')
		expect(vm.panelAccess).toBeNull()
		expect(vm.isDirty).toBe(false)
	})

	it('una línea inválida no se manda y queda en el campo', async () => {
		const vm = await mountViewModel(PHASE_3)

		vm.setCidrs('198.51.100.0/24\n10.0.0/8')
		await vm.save()

		expect(mocks.updateSecurity).not.toHaveBeenCalled()
		expect(vm.cidrsError).toBe(m.admin_error_cidr({ line: 2, value: '10.0.0/8' }))
		vm.setCidrs('10.0.0.0/8')
		expect(vm.cidrsError).toBeNull()
	})

	it('si la lista deja afuera la IP de la sesión, pide confirmación', async () => {
		const vm = await mountViewModel(PHASE_3)

		vm.setCidrs('10.0.0.0/8')
		expect(vm.clientIpAllowed).toBe(false)
		expect(vm.clientIpHint).toBe(m.admin_security_client_ip_out({ ip: '198.51.100.17' }))
		await vm.save()

		expect(vm.confirmingLockout).toBe(true)
		expect(mocks.updateSecurity).not.toHaveBeenCalled()

		await vm.confirmLockout()

		expect(mocks.updateSecurity).toHaveBeenCalledWith({ allowedCidrs: '10.0.0.0/8' })
		expect(vm.confirmingLockout).toBe(false)
	})

	it('sin saber la IP de la sesión, una lista nueva o distinta también pide confirmación', async () => {
		const vm = await mountViewModel({
			...PHASE_3,
			panelAccess: { allowedCidrs: [], clientIp: null },
		})

		vm.setCidrs('10.0.0.0/8')
		expect(vm.clientIpHint).toBeNull()
		await vm.save()

		expect(vm.confirmingLockout).toBe(true)
		expect(vm.lockoutTitle).toBe(m.admin_security_lockout_unknown_title())
		expect(vm.lockoutBody).toBe(m.admin_security_lockout_unknown_body())
		expect(mocks.updateSecurity).not.toHaveBeenCalled()

		await vm.confirmLockout()
		expect(mocks.updateSecurity).toHaveBeenCalledWith({ allowedCidrs: '10.0.0.0/8' })

		// Cambiar una lista que ya tenía rangos, también.
		vm.setCidrs('203.0.113.0/24')
		await vm.save()
		expect(vm.confirmingLockout).toBe(true)
	})

	it('vaciar la lista abre el panel a todos: no pide confirmación', async () => {
		const vm = await mountViewModel({
			...PHASE_3,
			panelAccess: { allowedCidrs: ['10.0.0.0/8'], clientIp: null },
		})

		vm.setCidrs('')
		await vm.save()

		expect(vm.confirmingLockout).toBe(false)
		expect(mocks.updateSecurity).toHaveBeenCalledWith({ allowedCidrs: '' })
	})

	it('con la IP conocida y afuera, el aviso dice cuál es', async () => {
		const vm = await mountViewModel(PHASE_3)

		vm.setCidrs('10.0.0.0/8')
		await vm.save()

		expect(vm.lockoutTitle).toBe(m.admin_security_lockout_title())
		expect(vm.lockoutBody).toBe(m.admin_security_lockout_body({ ip: '198.51.100.17' }))
	})

	it('si guardar falla desde la confirmación, el error queda adentro y sigue abierta', async () => {
		const vm = await mountViewModel(PHASE_3)
		vm.setCidrs('10.0.0.0/8')
		await vm.save()
		mocks.updateSecurity.mockRejectedValueOnce(new UnavailableError('caído'))

		await vm.confirmLockout()

		expect(vm.confirmingLockout).toBe(true)
		expect(vm.lockoutError).toBe(m.common_unavailable())
		vm.cancelLockout()
		expect(vm.lockoutError).toBeNull()
	})

	it('un 422 del backend en las IPs va al campo', async () => {
		const vm = await mountViewModel(PHASE_3)
		mocks.updateSecurity.mockRejectedValueOnce(
			new ValidationError('demasiados rangos', 'panelAllowedCidrs'),
		)

		vm.setCidrs('198.51.100.0/24\n203.0.113.0/24')
		await vm.save()

		expect(vm.cidrsError).toBe('demasiados rangos')
	})
})
