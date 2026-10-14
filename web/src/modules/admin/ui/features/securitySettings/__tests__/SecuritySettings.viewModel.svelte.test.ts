import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toast } from '@/shared/ui/stores'
import type { SecuritySettingsViewModel } from '..'
import { UnavailableError, ValidationError } from '@/shared/domain'
import { cidrListAllows, type Settings, TotpRequiredFirst, WouldLockOut } from '../../../../domain'
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
		removeCloudflareToken: vi.fn(),
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

// Como el backend: una lista que deja fuera `clientIp` es `WouldLockOut`, salvo con
// `confirmLockout`. Vacía, entra cualquier IP.
function leavesOut(cidrs: string, ip: string): boolean {
	return cidrs.trim() !== '' && ip !== '' && !cidrListAllows(cidrs, ip)
}

async function mountViewModel(initial: Settings): Promise<SecuritySettingsViewModel> {
	const client = createQueryClient({ retry: false })
	let current = initial
	mocks.updateSecurity.mockImplementation(
		(input: { require2fa?: boolean; allowedCidrs?: string; confirmLockout?: boolean }) => {
			const ip = current.panelAccess?.clientIp ?? ''
			if (
				input.allowedCidrs !== undefined &&
				!input.confirmLockout &&
				leavesOut(input.allowedCidrs, ip)
			) {
				return Promise.reject(new WouldLockOut())
			}
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

	it('el hint dice si la IP de la sesión queda en la lista', async () => {
		const vm = await mountViewModel(PHASE_3)

		vm.setCidrs('10.0.0.0/8')
		expect(vm.clientIpAllowed).toBe(false)
		expect(vm.clientIpHint).toBe(m.admin_security_client_ip_out({ ip: '198.51.100.17' }))
		vm.setCidrs('')
		expect(vm.clientIpHint).toBeNull()
	})

	it('si el backend dice would_lock_out, pide confirmar y reenvía con confirmLockout', async () => {
		const success = vi.spyOn(toast, 'success')
		const vm = await mountViewModel(PHASE_3)

		vm.setCidrs('10.0.0.0/8')
		await vm.save()

		// Lo decide el backend: el panel manda sin confirmar y espera el 409.
		expect(mocks.updateSecurity).toHaveBeenCalledWith({ allowedCidrs: '10.0.0.0/8' })
		expect(vm.confirmingLockout).toBe(true)
		expect(vm.lockoutBody).toBe(m.admin_security_lockout_body({ ip: '198.51.100.17' }))
		expect(success).not.toHaveBeenCalled()

		await vm.confirmLockout()

		expect(mocks.updateSecurity).toHaveBeenLastCalledWith({
			allowedCidrs: '10.0.0.0/8',
			confirmLockout: true,
		})
		expect(vm.confirmingLockout).toBe(false)
		expect(vm.isDirty).toBe(false)
		expect(success).toHaveBeenCalledWith(m.admin_security_saved())
	})

	it('confirmar reenvía todo lo que cambió, también el 2FA (el 409 no guardó nada)', async () => {
		const vm = await mountViewModel({ ...PHASE_3, require2fa: true })

		vm.setRequire2fa(false)
		vm.setCidrs('10.0.0.0/8')
		await vm.save()
		await vm.confirmLockout()

		expect(mocks.updateSecurity).toHaveBeenLastCalledWith({
			require2fa: false,
			allowedCidrs: '10.0.0.0/8',
			confirmLockout: true,
		})
	})

	it('si la lista incluye la IP de la sesión, se guarda sin confirmar', async () => {
		const vm = await mountViewModel(PHASE_3)

		vm.setCidrs('198.51.100.0/24\n10.0.0.0/8')
		await vm.save()

		expect(vm.confirmingLockout).toBe(false)
		expect(mocks.updateSecurity).toHaveBeenCalledTimes(1)
	})

	it('sin la IP de la sesión, el aviso no la nombra', async () => {
		const vm = await mountViewModel({
			...PHASE_3,
			panelAccess: { allowedCidrs: [], clientIp: '' },
		})
		mocks.updateSecurity.mockRejectedValueOnce(new WouldLockOut())

		vm.setCidrs('10.0.0.0/8')
		expect(vm.clientIpHint).toBeNull()
		await vm.save()

		expect(vm.confirmingLockout).toBe(true)
		expect(vm.lockoutBody).toBe(m.admin_security_lockout_body_no_ip())
	})

	it('cancelar la confirmación no guarda nada y deja lo escrito', async () => {
		const vm = await mountViewModel(PHASE_3)
		vm.setCidrs('10.0.0.0/8')
		await vm.save()

		vm.cancelLockout()

		expect(vm.confirmingLockout).toBe(false)
		expect(vm.cidrs).toBe('10.0.0.0/8')
		expect(mocks.updateSecurity).toHaveBeenCalledTimes(1)
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
