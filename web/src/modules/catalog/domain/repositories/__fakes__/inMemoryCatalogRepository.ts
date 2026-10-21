import type {
	App,
	AppChanges,
	AppRef,
	AppSlug,
	CatalogRepository,
	NewApp,
	NewProject,
	Project,
	ProjectSlug,
	ServiceApp,
	Stage,
	StageSlug,
} from '../..'
import {
	AppNotFound,
	KindMismatch,
	newServiceSpec,
	ProjectNotEmpty,
	ProjectNotFound,
	SlugTaken,
	StageNotEmpty,
	StageNotFound,
	summarizeCompose,
} from '../..'

const T0 = new Date('2026-10-01T12:00:00.000Z')

export function fakeProject(slug: string, name = slug): Project {
	return { slug: slug as ProjectSlug, name, createdAt: T0 }
}

export function fakeStage(project: string, slug: string): Stage {
	return { project: project as ProjectSlug, slug: slug as StageSlug, createdAt: T0 }
}

export function fakeServiceApp(
	project: string,
	stage: string,
	slug: string,
	overrides: Partial<Omit<ServiceApp, 'kind'>> = {},
): ServiceApp {
	return {
		kind: 'service',
		project: project as ProjectSlug,
		stage: stage as StageSlug,
		slug: slug as AppSlug,
		spec: newServiceSpec('nginx:1.27-alpine', 80),
		deployTimeoutSeconds: 300,
		configChangedAt: T0,
		lastDeployedAt: null,
		pendingChanges: false,
		createdAt: T0,
		services: [{ name: slug, ports: [80] }],
		...overrides,
	}
}

export function fakeComposeApp(
	project: string,
	stage: string,
	slug: string,
	composeYaml = 'services:\n  web:\n    image: nginx\n',
): App {
	return {
		kind: 'compose',
		project: project as ProjectSlug,
		stage: stage as StageSlug,
		slug: slug as AppSlug,
		composeYaml,
		deployTimeoutSeconds: 300,
		configChangedAt: T0,
		lastDeployedAt: null,
		pendingChanges: false,
		createdAt: T0,
		services: summarizeCompose(composeYaml).map((service) => ({
			name: service.name,
			ports: [],
		})),
	}
}

export interface FakeCatalogSeed {
	projects?: Project[]
	stages?: Stage[]
	apps?: App[]
}

// Un catálogo en memoria con las reglas que le importan al panel: slugs únicos, borrar
// solo lo que está vacío, y spec o compose según el tipo de la app.
export class InMemoryCatalogRepository implements CatalogRepository {
	projects: Project[]
	stages: Stage[]
	apps: App[]
	readonly calls = {
		listProjects: 0,
		listStages: 0,
		listApps: 0,
		createProject: 0,
		renameProject: 0,
		deleteProject: 0,
		createStage: 0,
		deleteStage: 0,
		getApp: 0,
		createApp: 0,
		updateApp: 0,
		deleteApp: 0,
	}
	/** El último cambio que llegó a `updateApp`, para ver qué se mandó. */
	lastChanges: AppChanges | null = null

	constructor(seed: FakeCatalogSeed = {}) {
		this.projects = [...(seed.projects ?? [])]
		this.stages = [...(seed.stages ?? [])]
		this.apps = [...(seed.apps ?? [])]
	}

	#project(slug: string): Project {
		const project = this.projects.find((item) => item.slug === slug)
		if (!project) throw new ProjectNotFound()
		return project
	}

	#stage(project: string, slug: string): Stage {
		const stage = this.stages.find((item) => item.project === project && item.slug === slug)
		if (!stage) throw new StageNotFound()
		return stage
	}

	#appIndex(ref: AppRef): number {
		const index = this.apps.findIndex(
			(app) => app.project === ref.project && app.stage === ref.stage && app.slug === ref.app,
		)
		if (index === -1) throw new AppNotFound()
		return index
	}

	async listProjects(): Promise<Project[]> {
		this.calls.listProjects += 1
		return [...this.projects]
	}

	async createProject(input: NewProject): Promise<Project> {
		this.calls.createProject += 1
		if (this.projects.some((item) => item.slug === input.slug)) throw new SlugTaken('project')
		const project = { ...input, createdAt: T0 }
		this.projects.push(project)
		return project
	}

	async renameProject(slug: ProjectSlug, name: string): Promise<Project> {
		this.calls.renameProject += 1
		const project = { ...this.#project(slug), name }
		this.projects = this.projects.map((item) => (item.slug === slug ? project : item))
		return project
	}

	async deleteProject(slug: ProjectSlug): Promise<void> {
		this.calls.deleteProject += 1
		this.#project(slug)
		if (this.stages.some((stage) => stage.project === slug)) throw new ProjectNotEmpty()
		this.projects = this.projects.filter((item) => item.slug !== slug)
	}

	async listStages(project: ProjectSlug): Promise<Stage[]> {
		this.calls.listStages += 1
		this.#project(project)
		return this.stages.filter((stage) => stage.project === project)
	}

	async createStage(project: ProjectSlug, slug: StageSlug): Promise<Stage> {
		this.calls.createStage += 1
		this.#project(project)
		if (this.stages.some((stage) => stage.project === project && stage.slug === slug)) {
			throw new SlugTaken('stage')
		}
		const stage = { project, slug, createdAt: T0 }
		this.stages.push(stage)
		return stage
	}

	async deleteStage(project: ProjectSlug, slug: StageSlug): Promise<void> {
		this.calls.deleteStage += 1
		this.#stage(project, slug)
		if (this.apps.some((app) => app.project === project && app.stage === slug)) {
			throw new StageNotEmpty()
		}
		this.stages = this.stages.filter(
			(stage) => !(stage.project === project && stage.slug === slug),
		)
	}

	async listApps(project: ProjectSlug, stage: StageSlug): Promise<App[]> {
		this.calls.listApps += 1
		this.#stage(project, stage)
		return this.apps.filter((app) => app.project === project && app.stage === stage)
	}

	async getApp(ref: AppRef): Promise<App> {
		this.calls.getApp += 1
		const app = this.apps[this.#appIndex(ref)]
		if (!app) throw new AppNotFound()
		return app
	}

	async createApp(input: NewApp): Promise<App> {
		this.calls.createApp += 1
		this.#stage(input.project, input.stage)
		const ref = { project: input.project, stage: input.stage, app: input.slug }
		if (
			this.apps.some(
				(app) =>
					app.project === ref.project && app.stage === ref.stage && app.slug === ref.app,
			)
		) {
			throw new SlugTaken('app')
		}
		const base = {
			project: input.project,
			stage: input.stage,
			slug: input.slug,
			deployTimeoutSeconds: input.deployTimeoutSeconds ?? 300,
			configChangedAt: T0,
			lastDeployedAt: null,
			pendingChanges: false,
			createdAt: T0,
		}
		const app: App =
			input.kind === 'service'
				? fakeServiceApp(input.project, input.stage, input.slug, {
						...base,
						spec: input.spec,
						services: [{ name: input.slug, ports: [input.spec.port] }],
					})
				: {
						...fakeComposeApp(
							input.project,
							input.stage,
							input.slug,
							input.composeYaml,
						),
						...base,
					}
		this.apps.push(app)
		return app
	}

	async updateApp(ref: AppRef, changes: AppChanges): Promise<App> {
		this.calls.updateApp += 1
		this.lastChanges = changes
		const index = this.#appIndex(ref)
		const current = this.apps[index]
		if (!current) throw new AppNotFound()
		if (changes.spec && current.kind !== 'service') throw new KindMismatch()
		if (changes.composeYaml !== undefined && current.kind !== 'compose')
			throw new KindMismatch()

		const configChanged = changes.spec !== undefined || changes.composeYaml !== undefined
		const configChangedAt = configChanged ? new Date() : current.configChangedAt
		const common = {
			deployTimeoutSeconds: changes.deployTimeoutSeconds ?? current.deployTimeoutSeconds,
			configChangedAt,
			pendingChanges:
				current.lastDeployedAt !== null && configChangedAt > current.lastDeployedAt,
		}
		// Los servicios salen de la configuración guardada, como en el backend (§2.12).
		let next: App
		if (current.kind === 'service') {
			const spec = changes.spec ?? current.spec
			next = {
				...current,
				...common,
				spec,
				services: [{ name: current.slug, ports: [spec.port] }],
			}
		} else {
			const composeYaml = changes.composeYaml ?? current.composeYaml
			const { services } = fakeComposeApp(
				current.project,
				current.stage,
				current.slug,
				composeYaml,
			)
			next = { ...current, ...common, composeYaml, services }
		}
		this.apps[index] = next
		return next
	}

	async deleteApp(ref: AppRef): Promise<void> {
		this.calls.deleteApp += 1
		const index = this.#appIndex(ref)
		this.apps.splice(index, 1)
	}
}
