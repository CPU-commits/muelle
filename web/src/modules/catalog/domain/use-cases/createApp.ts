import type { App } from '../entities/app'
import type { CatalogDeps, NewApp } from '../repositories/catalogRepository'
import { type AppSpec, parseAppSpec } from '../value-objects/appSpec'
import { parseComposeYaml } from '../value-objects/composeYaml'
import { parseDeployTimeout } from '../value-objects/deployTimeout'
import { parseAppSlug, type ProjectSlug, type StageSlug } from '../value-objects/slugs'

export type CreateAppInput = {
	project: ProjectSlug
	stage: StageSlug
	slug: string
	deployTimeoutSeconds?: number
} & ({ kind: 'service'; spec: AppSpec } | { kind: 'compose'; composeYaml: string })

/** Crea la app (solo el registro: desplegarla es de la Fase 2). `SlugTaken` si ya existe. */
export type CreateApp = (input: CreateAppInput) => Promise<App>

export function makeCreateApp({ catalog }: CatalogDeps): CreateApp {
	return async (input) => {
		const base = {
			project: input.project,
			stage: input.stage,
			slug: parseAppSlug(input.slug),
			...(input.deployTimeoutSeconds === undefined
				? {}
				: { deployTimeoutSeconds: parseDeployTimeout(input.deployTimeoutSeconds) }),
		}
		const app: NewApp =
			input.kind === 'service'
				? { ...base, kind: 'service', spec: parseAppSpec(input.spec) }
				: { ...base, kind: 'compose', composeYaml: parseComposeYaml(input.composeYaml) }
		return catalog.createApp(app)
	}
}
