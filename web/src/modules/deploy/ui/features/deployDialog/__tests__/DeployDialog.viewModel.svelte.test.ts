import { beforeEach, describe, expect, it, vi } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toasts } from '@/shared/ui/stores'
import { UnavailableError, ValidationError } from '@/shared/domain'
import { asDeployId } from '../../../../domain'
import { APP } from '../../../../domain/repositories/__fakes__/deployFixtures'
import { mountViewModel } from '../../../__tests__/mountViewModel'
import { type DeployDialogProps, DeployDialogViewModel } from '../DeployDialog.viewModel.svelte.ts'

const mocks = vi.hoisted(() => ({ createDeploy: vi.fn() }))

vi.mock('@/modules/deploy/container', () => ({
	deployUseCases: {
		getAppStatus: vi.fn(),
		listDeploys: vi.fn(),
		getDeploy: vi.fn(),
		createDeploy: mocks.createDeploy,
		rollback: vi.fn(),
		followDeployLog: vi.fn(),
		streamAppLogs: vi.fn(),
	},
}))

function props(overrides: Partial<DeployDialogProps> = {}): DeployDialogProps {
	return {
		app: APP,
		kind: null,
		services: ['api'],
		currentImages: { api: 'ghcr.io/auditfy/api:sha-3f9c2e1' },
		onDeployed: vi.fn(),
		onClose: vi.fn(),
		...overrides,
	}
}

describe('DeployDialogViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.createDeploy.mockResolvedValue({ id: asDeployId(145) })
	})

	it('un campo por servicio, con la imagen actual de placeholder', async () => {
		const { vm } = await mountViewModel(
			() => new DeployDialogViewModel(() => props({ services: ['gateway', 'runner'] })),
		)
		expect(vm.fields).toEqual(['gateway', 'runner'])

		const single = await mountViewModel(() => new DeployDialogViewModel(() => props()))
		expect(single.vm.fields).toEqual(['api'])
		expect(single.vm.placeholder('api')).toBe('ghcr.io/auditfy/api:sha-3f9c2e1')
	})

	it('vacío redespliega: encola, avisa y devuelve el id', async () => {
		const input = props()
		const { vm } = await mountViewModel(() => new DeployDialogViewModel(() => input))

		await vm.submit()

		expect(mocks.createDeploy).toHaveBeenCalledWith({
			app: APP,
			kind: null,
			services: ['api'],
			overrides: {},
		})
		expect(input.onDeployed).toHaveBeenCalledWith(145)
		expect(toasts.items.some((item) => item.title === m.deploy_toast_queued({ id: 145 }))).toBe(
			true,
		)
	})

	it('con una imagen la manda y después limpia el formulario', async () => {
		const { vm } = await mountViewModel(() => new DeployDialogViewModel(() => props()))
		vm.setOverride('api', 'ghcr.io/auditfy/api:sha-1')

		await vm.submit()

		expect(mocks.createDeploy.mock.calls[0]?.[0]).toMatchObject({
			overrides: { api: 'ghcr.io/auditfy/api:sha-1' },
		})
		expect(vm.overrides).toEqual({})
	})

	it('una imagen inválida se marca en su campo sin llamar al backend', async () => {
		const { vm } = await mountViewModel(() => new DeployDialogViewModel(() => props()))
		vm.setOverride('api', 'no vale')

		await vm.submit()

		expect(vm.errors.api).toBe(m.deploy_dialog_invalid_image())
		expect(mocks.createDeploy).not.toHaveBeenCalled()

		vm.setOverride('api', 'ghcr.io/auditfy/api:sha-1')
		expect(vm.errors.api).toBeUndefined()
	})

	it('un 422 del backend (images.<servicio>) va al campo de ese servicio', async () => {
		mocks.createDeploy.mockRejectedValue(new ValidationError('no existe', 'images.runner'))
		const { vm } = await mountViewModel(
			() => new DeployDialogViewModel(() => props({ services: ['gateway', 'runner'] })),
		)
		vm.setOverride('runner', 'ghcr.io/x/runner:v2')

		await vm.submit()

		expect(vm.errors.runner).toBe(m.deploy_dialog_invalid_image())
		expect(vm.formError).toBeNull()
	})

	it('otro error va al banner del diálogo', async () => {
		mocks.createDeploy.mockRejectedValue(
			new UnavailableError('Llega con la Fase 2', { phase: 2 }),
		)
		const input = props()
		const { vm } = await mountViewModel(() => new DeployDialogViewModel(() => input))

		await vm.submit()

		expect(vm.formError).toBe(m.common_coming_title({ phase: 2 }))
		expect(input.onDeployed).not.toHaveBeenCalled()

		vm.close()
		expect(vm.formError).toBeNull()
		expect(input.onClose).toHaveBeenCalled()
	})
})
