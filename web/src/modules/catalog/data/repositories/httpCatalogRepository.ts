import type { ApiError } from '@/shared/data/http/errors'
import { type DomainErrorMap, withDomainErrors } from '@/shared/data/http/domainErrors'
import type { CatalogRepository } from '../../domain'
import {
	AppDeployStopping,
	AppHasDomains,
	AppNotFound,
	KindMismatch,
	ProjectNotEmpty,
	ProjectNotFound,
	SlugTaken,
	StageNetworkConflict,
	StageNotEmpty,
	StageNotFound,
} from '../../domain'
import { catalogHttpDataSource as ds } from '../datasources/catalogHttpDataSource'
import {
	appFromDto,
	createAppPayload,
	createProjectPayload,
	projectFromDto,
	stageFromDto,
	updateAppPayload,
} from '../mappers/catalogMapper'

const cause = (error: ApiError) => ({ cause: error })

// Los campos de un 422 con su nombre del dominio. Los de volúmenes (`volumes[0].mount`)
// y los que se llaman igual pasan tal cual.
const FIELDS: Readonly<Record<string, string>> = {
	'healthcheck.interval_s': 'healthcheck.intervalSeconds',
	'healthcheck.timeout_s': 'healthcheck.timeoutSeconds',
	'healthcheck.start_period_s': 'healthcheck.startPeriodSeconds',
	'resources.memory_mb': 'resources.memoryMb',
	compose_yaml: 'composeYaml',
	deploy_timeout_s: 'deployTimeoutSeconds',
}

const PROJECT: DomainErrorMap = {
	notFound: (error) => new ProjectNotFound(cause(error)),
	fields: FIELDS,
	codes: {
		conflict: (error) => new SlugTaken('project', cause(error)),
		not_empty: (error) => new ProjectNotEmpty(cause(error)),
	},
}

// Un 404 al crear un stage es su proyecto; al borrarlo, el stage (o su proyecto).
// `network_conflict`: ya hay una red de Docker con el nombre del stage que no creó muelle
// (la crea ingress al crear el stage).
const NEW_STAGE: DomainErrorMap = {
	notFound: (error) => new ProjectNotFound(cause(error)),
	fields: FIELDS,
	codes: {
		conflict: (error) => new SlugTaken('stage', cause(error)),
		network_conflict: (error) => new StageNetworkConflict(cause(error)),
	},
}

const STAGE: DomainErrorMap = {
	notFound: (error) => new StageNotFound(cause(error)),
	fields: FIELDS,
	codes: {
		conflict: (error) => new SlugTaken('app', cause(error)),
		not_empty: (error) => new StageNotEmpty(cause(error)),
	},
}

const APP: DomainErrorMap = {
	notFound: (error) => new AppNotFound(cause(error)),
	fields: FIELDS,
	codes: { kind_mismatch: (error) => new KindMismatch(cause(error)) },
}

// Borrar una app: `not_empty` si tiene dominios; `conflict` si un deploy suyo no se detuvo
// a tiempo y la app no se borró (contrato §1.1, `RemoveApp`).
const DELETE_APP: DomainErrorMap = {
	...APP,
	codes: {
		not_empty: (error) => new AppHasDomains(cause(error)),
		conflict: (error) => new AppDeployStopping(cause(error)),
	},
}

export const httpCatalogRepository: CatalogRepository = {
	listProjects: (signal) =>
		withDomainErrors(async () => (await ds.listProjects(signal)).map(projectFromDto), PROJECT),

	createProject: (input) =>
		withDomainErrors(
			async () => projectFromDto(await ds.createProject(createProjectPayload(input))),
			PROJECT,
		),

	renameProject: (project, name) =>
		withDomainErrors(
			async () => projectFromDto(await ds.renameProject(project, { name })),
			PROJECT,
		),

	deleteProject: (project) => withDomainErrors(() => ds.deleteProject(project), PROJECT),

	listStages: (project, signal) =>
		withDomainErrors(
			async () => (await ds.listStages(project, signal)).map(stageFromDto),
			PROJECT,
		),

	createStage: (project, slug) =>
		withDomainErrors(
			async () => stageFromDto(await ds.createStage(project, { slug })),
			NEW_STAGE,
		),

	deleteStage: (project, stage) => withDomainErrors(() => ds.deleteStage(project, stage), STAGE),

	listApps: (project, stage, signal) =>
		withDomainErrors(
			async () => (await ds.listApps(project, stage, signal)).map(appFromDto),
			STAGE,
		),

	getApp: (ref, signal) =>
		withDomainErrors(
			async () => appFromDto(await ds.getApp(ref.project, ref.stage, ref.app, signal)),
			APP,
		),

	createApp: (input) =>
		withDomainErrors(
			async () =>
				appFromDto(await ds.createApp(input.project, input.stage, createAppPayload(input))),
			STAGE,
		),

	updateApp: (ref, changes) =>
		withDomainErrors(
			async () =>
				appFromDto(
					await ds.updateApp(ref.project, ref.stage, ref.app, updateAppPayload(changes)),
				),
			APP,
		),

	deleteApp: (ref) =>
		withDomainErrors(() => ds.deleteApp(ref.project, ref.stage, ref.app), DELETE_APP),
}
