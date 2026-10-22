import { beforeEach, describe, expect, it, vi } from 'vitest'
import { page } from 'vitest/browser'
import { render } from 'vitest-browser-svelte'
import { createQueryClient } from '@/shared/data/query'
import { ValidationError } from '@/shared/domain'
import { appSummaryOf, DEFAULT_HEALTHCHECK_TIMINGS, type ServiceApp } from '../../../../domain'
import { fakeServiceApp } from '../../../../domain/repositories/__fakes__/inMemoryCatalogRepository'
import { serverFieldMessage } from '../../../components/catalogMessages'
import { SECTION_IDS } from '../ServiceConfig.viewModel.svelte.ts'
import ServiceConfigHarness from './ServiceConfigHarness.svelte'

const mocks = vi.hoisted(() => ({ updateApp: vi.fn() }))

vi.mock('@/modules/catalog/container', () => ({
	catalogUseCases: {
		listCatalog: vi.fn(),
		createProject: vi.fn(),
		renameProject: vi.fn(),
		deleteProject: vi.fn(),
		createStage: vi.fn(),
		deleteStage: vi.fn(),
		getApp: vi.fn(),
		createApp: vi.fn(),
		updateApp: mocks.updateApp,
		deleteApp: vi.fn(),
	},
}))
vi.mock('$app/state', () => ({ page: { url: new URL('http://localhost/configuration') } }))
vi.mock('$app/navigation', () => ({ beforeNavigate: vi.fn(), goto: vi.fn() }))

const app: ServiceApp = fakeServiceApp('auditfy', 'staging', 'api', {
	spec: {
		image: 'ghcr.io/aumi-app/backend-app:sha-3f9c2e1',
		port: 3000,
		replicas: 2,
		command: [],
		healthcheck: { type: 'http', path: '/health', ...DEFAULT_HEALTHCHECK_TIMINGS },
		volumes: [{ name: 'uploads', mount: '/app/uploads' }],
		resources: { memoryMb: 512, cpus: 1 },
	},
})

type Card = keyof typeof SECTION_IDS

// Un cambio cualquiera en la tarjeta, para que "Guardar" tenga algo que mandar.
const EDITS: Record<Exclude<Card, 'delete'>, () => Promise<void>> = {
	image: () => page.getByRole('textbox', { name: 'Imagen' }).fill('nginx:1.28'),
	run: () => page.getByLabelText('Réplicas').fill('3'),
	health: () => page.getByLabelText('Path').fill('/otro'),
	volumes: () => page.getByLabelText('Ruta del volumen 1').fill('/otro'),
	resources: () => page.getByLabelText('Timeout del deploy').fill('600'),
}

// Cada campo que el backend puede nombrar en un 422 (vo.AppSpec, en nombres del dominio),
// guardando desde cada tarjeta. Si ninguna parte de la tarjeta lo muestra, el error se
// pierde: es lo que esto vigila.
const CASES: [Exclude<Card, 'delete'>, string, string][] = [
	['image', 'image', 'la imagen no es una referencia válida de Docker'],
	['image', 'port', 'debe estar entre 1 y 65535'],
	['image', 'spec', 'JSON inválido: el campo "x" no existe'],
	['run', 'port', 'debe estar entre 1 y 65535'],
	['run', 'replicas', 'debe estar entre 0 y 50'],
	['run', 'command', 'máximo 64 argumentos'],
	['health', 'healthcheck', 'con type none no lleva path ni command'],
	['health', 'healthcheck.type', 'debe ser http, bun-http, command o none'],
	['health', 'healthcheck.path', 'debe empezar con /'],
	['health', 'healthcheck.command', 'solo aplica a type command'],
	['health', 'healthcheck.intervalSeconds', 'debe estar entre 1 y 3600'],
	['health', 'healthcheck.timeoutSeconds', 'debe estar entre 1 y 600'],
	['health', 'healthcheck.retries', 'debe estar entre 1 y 20'],
	['health', 'healthcheck.startPeriodSeconds', 'debe estar entre 1 y 3600'],
	['volumes', 'volumes', 'máximo 10'],
	['volumes', 'volumes[0].name', '"uploads" está repetido'],
	['volumes', 'volumes[0].mount', 'debe ser una ruta absoluta limpia'],
	['resources', 'resources', 'debe estar entre 16 y 1048576'],
	['resources', 'resources.memoryMb', 'debe estar entre 16 y 1048576'],
	['resources', 'resources.cpus', 'debe estar entre 0.01 y 256'],
	['resources', 'deployTimeoutSeconds', 'debe estar entre 30 y 3600'],
]

describe('ServiceConfig', () => {
	beforeEach(() => {
		vi.clearAllMocks()
	})

	it.each(CASES)('un 422 de %s → %s se ve en la tarjeta', async (card, field, backend) => {
		mocks.updateApp.mockRejectedValue(new ValidationError(backend, field))
		await render(ServiceConfigHarness, {
			client: createQueryClient({ retry: false }),
			app,
			summary: appSummaryOf(app),
		})

		await EDITS[card]()
		const section = page.elementLocator(
			document.getElementById(SECTION_IDS[card]) as HTMLElement,
		)
		await section.getByRole('button', { name: 'Guardar' }).click()

		expect(mocks.updateApp).toHaveBeenCalled()
		await expect
			.element(section.getByText(serverFieldMessage(field, backend), { exact: true }).first())
			.toBeVisible()
	})
})
