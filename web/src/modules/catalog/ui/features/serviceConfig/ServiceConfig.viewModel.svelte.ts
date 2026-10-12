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
	readonly #volumeCount

	constructor(app: () => ServiceApp) {
		this.#app = app
		this.image = createSectionForm(app, imageSection)
		this.run = createSectionForm(app, runSection)
		this.health = createSectionForm(app, healthSection)
		this.volumes = createSectionForm(app, volumesSection)
		this.resources = createSectionForm(app, resourcesSection)
		this.#healthValues = this.health.form.useSelector((state) => state.values)
		this.#volumeCount = this.volumes.form.useSelector((state) => state.values.volumes.length)
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

	/** Lo que va a ejecutar Swarm, con los valores que se están editando. */
	get healthPreview(): string | null {
		const { healthcheck } = healthcheckFrom(this.#healthValues.current, sectionMessages)
		return healthcheckPreview(healthcheck, this.#app().spec.port)
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
