import { beforeEach, describe, expect, it, vi } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { UnavailableError } from '@/shared/domain'
import { AppRemoving, asDeployId, NoPreviousDeploy } from '../../../../domain'
import { APP, makeDeploy } from '../../../../domain/repositories/__fakes__/deployFixtures'
import { mountViewModel } from '../../../__tests__/mountViewModel'
import {
	type RollbackDialogProps,
	RollbackDialogViewModel,
} from '../RollbackDialog.viewModel.svelte.ts'

const mocks = vi.hoisted(() => ({ rollback: vi.fn() }))

vi.mock('@/modules/deploy/container', () => ({
	deployUseCases: {
		getAppStatus: vi.fn(),
		listDeploys: vi.fn(),
		getDeploy: vi.fn(),
		createDeploy: vi.fn(),
		rollback: mocks.rollback,
		followDeployLog: vi.fn(),
		streamAppLogs: vi.fn(),
	},
}))

const TARGET = makeDeploy(140, {
	images: [{ service: 'api', ref: 'ghcr.io/aumi-app/backend-app:sha-e52b8f0', digest: null }],
})

function props(overrides: Partial<RollbackDialogProps> = {}): RollbackDialogProps {
	return {
		app: APP,
		target: TARGET,
		historyComplete: true,
		onDone: vi.fn(),
		onClose: vi.fn(),
		...overrides,
	}
}

describe('RollbackDialogViewModel', () => {
	beforeEach(() => {
		vi.clearAllMocks()
		mocks.rollback.mockResolvedValue({ id: asDeployId(145) })
	})

	it('muestra a dónde vuelve y encola el rollback', async () => {
		const input = props()
		const { vm } = await mountViewModel(() => new RollbackDialogViewModel(() => input))

		expect(vm.title).toBe(m.deploy_rollback_title({ id: 140 }))
		expect(vm.targetImages).toBe('backend-app:sha-e52b8f0')
		expect(vm.nothingToRollBack).toBe(false)

		await vm.confirm()

		expect(mocks.rollback).toHaveBeenCalledWith(APP)
		expect(input.onDone).toHaveBeenCalledWith(145)
	})

	it('un 409 (NoPreviousDeploy) dice claramente que no hay a dónde volver', async () => {
		mocks.rollback.mockRejectedValue(new NoPreviousDeploy())
		const input = props({ target: null, historyComplete: false })
		const { vm } = await mountViewModel(() => new RollbackDialogViewModel(() => input))
		expect(vm.nothingToRollBack).toBe(false)

		await vm.confirm()

		expect(vm.noPrevious).toBe(true)
		expect(vm.nothingToRollBack).toBe(true)
		expect(vm.formError).toBeNull()
		expect(input.onDone).not.toHaveBeenCalled()
	})

	it('con el historial entero y sin anterior, avisa antes y no llama al backend', async () => {
		const { vm } = await mountViewModel(
			() => new RollbackDialogViewModel(() => props({ target: null, historyComplete: true })),
		)

		expect(vm.nothingToRollBack).toBe(true)
		await vm.confirm()
		expect(mocks.rollback).not.toHaveBeenCalled()
	})

	it('una app que se está borrando (409 conflict) lo dice en el banner, no "no hay anterior"', async () => {
		mocks.rollback.mockRejectedValue(new AppRemoving())
		const { vm } = await mountViewModel(() => new RollbackDialogViewModel(() => props()))

		await vm.confirm()

		expect(vm.noPrevious).toBe(false)
		expect(vm.formError).toBe(m.deploy_error_app_removing())
	})

	it('otro error va al banner; cerrar lo limpia', async () => {
		mocks.rollback.mockRejectedValue(new UnavailableError('caído'))
		const input = props()
		const { vm } = await mountViewModel(() => new RollbackDialogViewModel(() => input))

		await vm.confirm()
		expect(vm.formError).toBe(m.common_unavailable())

		vm.close()
		expect(vm.formError).toBeNull()
		expect(input.onClose).toHaveBeenCalled()
	})
})
