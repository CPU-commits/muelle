import { beforeEach, describe, expect, it, vi } from 'vitest'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import { toasts } from '@/shared/ui/stores'
import { UnavailableError } from '@/shared/domain'
import { AppRemoving, asDeployId, ImageRejected } from '../../../../domain'
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

	it.each([
		['invalid', m.deploy_dialog_invalid_image()],
		['unknown_service', m.deploy_dialog_reason_unknown_service({ service: 'runner' })],
	] as const)(
		'un 422 de images.<servicio> (%s) va al campo de ese servicio, con su texto',
		async (reason, text) => {
			mocks.createDeploy.mockRejectedValue(new ImageRejected('motivo', 'runner', reason))
			const { vm } = await mountViewModel(
				() => new DeployDialogViewModel(() => props({ services: ['gateway', 'runner'] })),
			)
			vm.setOverride('runner', 'ghcr.io/x/runner:v2')

			await vm.submit()

			expect(vm.errors.runner).toBe(text)
			expect(vm.formError).toBeNull()
		},
	)

	it('un 422 de image (compose, o image con images) va al campo único con su texto', async () => {
		mocks.createDeploy.mockRejectedValueOnce(new ImageRejected('motivo', null, 'compose'))
		const { vm } = await mountViewModel(() => new DeployDialogViewModel(() => props()))
		vm.setOverride('api', 'ghcr.io/x/api:v2')
		await vm.submit()
		expect(vm.errors.api).toBe(m.deploy_dialog_reason_compose())

		mocks.createDeploy.mockRejectedValueOnce(new ImageRejected('motivo', null, 'both'))
		vm.setOverride('api', 'ghcr.io/x/api:v3')
		await vm.submit()
		expect(vm.errors.api).toBe(m.deploy_dialog_reason_both())
	})

	it('un servicio que el diálogo no muestra va al banner', async () => {
		mocks.createDeploy.mockRejectedValue(new ImageRejected('motivo', 'otro', 'unknown_service'))
		const { vm } = await mountViewModel(() => new DeployDialogViewModel(() => props()))
		vm.setOverride('api', 'ghcr.io/x/api:v2')

		await vm.submit()

		expect(vm.formError).toBe(m.deploy_dialog_reason_unknown_service({ service: 'otro' }))
	})

	it('una app que se está borrando (409 conflict) lo dice en el banner', async () => {
		mocks.createDeploy.mockRejectedValue(new AppRemoving())
		const { vm } = await mountViewModel(() => new DeployDialogViewModel(() => props()))

		await vm.submit()

		expect(vm.formError).toBe(m.deploy_error_app_removing())
	})

	it('si algo sigue fijado por un rollback, avisa que sin imagen se repite esa versión', async () => {
		const normal = await mountViewModel(() => new DeployDialogViewModel(() => props()))
		expect(normal.vm.pinnedTitle).toBeNull()

		const pin = { rollback: { id: asDeployId(144), services: ['api'] }, others: [] }
		const single = await mountViewModel(
			() => new DeployDialogViewModel(() => props({ pinned: pin })),
		)
		expect(single.vm.pinnedTitle).toBe(m.deploy_dialog_rollback_pin_title({ id: 144 }))

		// En una compose dice cuáles: un override de un servicio suelta solo ese.
		const compose = await mountViewModel(
			() =>
				new DeployDialogViewModel(() =>
					props({
						kind: 'compose',
						services: ['api', 'web', 'worker'],
						pinned: {
							rollback: { id: asDeployId(144), services: ['web', 'worker'] },
							others: [],
						},
					}),
				),
		)
		expect(compose.vm.pinnedTitle).toBe(
			m.deploy_dialog_rollback_pin_services_title({ id: 144, services: 'web, worker' }),
		)
	})

	it('unos fijados por el rollback y otros por otra cosa: nombra el rollback y los otros aparte', async () => {
		const { vm } = await mountViewModel(
			() =>
				new DeployDialogViewModel(() =>
					props({
						kind: 'compose',
						services: ['api', 'web', 'worker'],
						pinned: {
							rollback: { id: asDeployId(144), services: ['web'] },
							others: ['api', 'worker'],
						},
					}),
				),
		)
		expect(vm.pinnedTitle).toBe(
			m.deploy_dialog_mixed_pin_services_title({
				id: 144,
				rollback: 'web',
				services: 'api, worker',
			}),
		)
		expect(vm.pinnedTitle).toContain('#144')
		expect(vm.pinnedTitle).toContain('api, worker')
	})

	it('fijado en un digest sin un rollback a la vista (salió del historial, o un override): lo dice igual', async () => {
		const single = await mountViewModel(
			() =>
				new DeployDialogViewModel(() =>
					props({ pinned: { rollback: null, others: ['api'] } }),
				),
		)
		expect(single.vm.pinnedTitle).toBe(m.deploy_dialog_digest_pin_title())

		const compose = await mountViewModel(
			() =>
				new DeployDialogViewModel(() =>
					props({
						kind: 'compose',
						services: ['api', 'web', 'worker'],
						pinned: { rollback: null, others: ['web'] },
					}),
				),
		)
		expect(compose.vm.pinnedTitle).toBe(
			m.deploy_dialog_digest_pin_services_title({ services: 'web' }),
		)
	})

	it('el texto bajo el aviso sirve para cualquier digest fijo: un rollback, un override o la configuración', async () => {
		const normal = await mountViewModel(() => new DeployDialogViewModel(() => props()))
		expect(normal.vm.pinnedBody).toBeNull()

		// Un digest fijado a propósito en la configuración: nada de "volver a seguir un tag".
		const config = await mountViewModel(
			() =>
				new DeployDialogViewModel(() =>
					props({ pinned: { rollback: null, others: ['api'] } }),
				),
		)
		expect(config.vm.pinnedBody).toBe(m.deploy_dialog_pinned_body())
		const rollback = await mountViewModel(
			() =>
				new DeployDialogViewModel(() =>
					props({
						pinned: {
							rollback: { id: asDeployId(144), services: ['api'] },
							others: [],
						},
					}),
				),
		)
		expect(rollback.vm.pinnedBody).toBe(m.deploy_dialog_pinned_body())
		for (const locale of ['es', 'en'] as const) {
			expect(m.deploy_dialog_pinned_body({}, { locale })).not.toMatch(
				/volver a seguir|again|rollback/i,
			)
		}
	})

	it('otro error va al banner del diálogo', async () => {
		mocks.createDeploy.mockRejectedValue(new UnavailableError('caído', { reason: 'server' }))
		const input = props()
		const { vm } = await mountViewModel(() => new DeployDialogViewModel(() => input))

		await vm.submit()

		expect(vm.formError).toBe(m.common_server_error())
		expect(input.onDeployed).not.toHaveBeenCalled()

		vm.close()
		expect(vm.formError).toBeNull()
		expect(input.onClose).toHaveBeenCalled()
	})
})
