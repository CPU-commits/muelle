import { page } from '$app/state'
import type { NavItem } from '@/shared/ui'
import { m } from '@/shared/ui/i18n/paraglide/messages'
import {
	type AppRef,
	appRefOf,
	type HealthcheckType,
	healthcheckPreview,
	SPEC_LIMITS,
	type ServiceApp,
} from '../../../domain'
import { guardUnsavedChanges } from '../../components/unsavedGuard'
import { createSectionForm, sectionMessages } from './sectionForm.svelte.ts'
import {
	EMPTY_VOLUME,
	healthcheckFrom,
	healthSection,
	imageSection,
	resourcesSection,
	runSection,
	volumesSection,
} from './sections'

/** Los ids de las tarjetas: la subnav de la izquierda salta a ellas. */
export const SECTION_IDS = {
	image: 'c-imagen',
	run: 'c-ejecucion',
	health: 'c-health',
	volumes: 'c-vol',
	resources: 'c-res',
	delete: 'c-del',
} as const

const HEALTH_REQUIREMENT: Record<HealthcheckType, () => string> = {
	http: () => m.catalog_config_health_req_http(),
	'bun-http': () => m.catalog_config_health_req_bun(),
	command: () => m.catalog_config_health_req_command(),
	none: () => m.catalog_config_health_req_none(),
}

// App·Configuración de una app de formulario (`App-Configuracion.dc.html`): una tarjeta
// por parte del spec, cada una con su formulario y su "Guardar".
export class ServiceConfigViewModel {
	readonly image
	readonly run
	readonly health
	readonly volumes
	readonly resources

	readonly #app: () => ServiceApp
	readonly #healthValues
	readonly #editedPort
	readonly #volumeCount
	readonly #pristine

	constructor(app: () => ServiceApp) {
		this.#app = app
		this.image = createSectionForm(app, imageSection)
		this.run = createSectionForm(app, runSection)
		this.health = createSectionForm(app, healthSection)
		this.volumes = createSectionForm(app, volumesSection)
		this.resources = createSectionForm(app, resourcesSection)
		this.#healthValues = this.health.form.useSelector((state) => state.values)
		this.#editedPort = this.run.form.useSelector((state) => state.values.port)
		this.#volumeCount = this.volumes.form.useSelector((state) => state.values.volumes.length)
		// Una tarjeta con lo que tenía al cargar (o al guardar) no cuenta como cambio.
		this.#pristine = [this.image, this.run, this.health, this.volumes, this.resources].map(
			(section) => section.form.useSelector((state) => state.isDefaultValue),
		)
		guardUnsavedChanges(() => this.isDirty)
	}

	/** Alguna tarjeta tiene cambios sin guardar. */
	get isDirty(): boolean {
		return this.#pristine.some((pristine) => !pristine.current)
	}

	readonly healthOptions = [
		{ value: 'http', label: m.catalog_config_health_http() },
		{ value: 'bun-http', label: m.catalog_config_health_bun() },
		{ value: 'command', label: m.catalog_config_health_command() },
		{ value: 'none', label: m.catalog_config_health_none() },
	] as const satisfies readonly { value: HealthcheckType; label: string }[]

	get ref(): AppRef {
		return appRefOf(this.#app())
	}

	get navItems(): NavItem[] {
		const hash = page.url.hash
		const items: [string, string][] = [
			[SECTION_IDS.image, m.catalog_config_image()],
			[SECTION_IDS.run, m.catalog_config_run()],
			[SECTION_IDS.health, m.catalog_config_health()],
			[SECTION_IDS.volumes, m.catalog_config_volumes()],
			[SECTION_IDS.resources, m.catalog_config_resources()],
			[SECTION_IDS.delete, m.catalog_delete_app_title()],
		]
		return items.map(([id, label], index) => ({
			href: `#${id}`,
			label,
			active: hash ? hash === `#${id}` : index === 0,
		}))
	}

	get healthType(): HealthcheckType {
		return this.#healthValues.current.type
	}

	/**
	 * Lo que va a ejecutar Swarm, con lo que se está editando: el healthcheck y el puerto de
	 * Ejecución (si todavía no es un puerto válido, el guardado).
	 */
	get healthPreview(): string | null {
		const { healthcheck } = healthcheckFrom(this.#healthValues.current, sectionMessages)
		const edited = Number(this.#editedPort.current)
		const port =
			Number.isInteger(edited) &&
			edited >= SPEC_LIMITS.port.min &&
			edited <= SPEC_LIMITS.port.max
				? edited
				: this.#app().spec.port
		return healthcheckPreview(healthcheck, port)
	}

	get healthRequirement(): string {
		return HEALTH_REQUIREMENT[this.healthType]()
	}

	get canAddVolume(): boolean {
		return this.#volumeCount.current < SPEC_LIMITS.maxVolumes
	}

	setHealthType = (type: HealthcheckType): void => {
		this.health.form.setFieldValue('type', type)
	}

	addVolume = (): void => {
		this.volumes.form.pushFieldValue('volumes', { ...EMPTY_VOLUME })
	}

	removeVolume = (index: number): void => {
		this.volumes.form.removeFieldValue('volumes', index)
	}
}
