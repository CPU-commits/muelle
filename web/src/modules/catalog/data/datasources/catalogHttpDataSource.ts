import { z } from 'zod'
import { http } from '@/shared/data/http/client'
import { parseOrThrow } from '@/shared/data/http/parse'
import {
	type AppDto,
	appDtoSchema,
	type CreateAppPayload,
	type CreateProjectPayload,
	type CreateStagePayload,
	type ProjectDto,
	projectDtoSchema,
	type RenameProjectPayload,
	type StageDto,
	stageDtoSchema,
	type UpdateAppPayload,
} from '../dtos/catalogDto'

// Las rutas del catálogo (catalog/infra/in/http/handler.go). Los slugs van escapados
// aunque el backend solo acepte [a-z0-9-]: un parámetro de la URL no es de fiar.
const projectPath = (project: string) => `/projects/${encodeURIComponent(project)}`
const stagePath = (project: string, stage: string) =>
	`${projectPath(project)}/stages/${encodeURIComponent(stage)}`
const appPath = (project: string, stage: string, app: string) =>
	`${stagePath(project, stage)}/apps/${encodeURIComponent(app)}`

const withSignal = (signal?: AbortSignal) => (signal ? { signal } : {})

export const catalogHttpDataSource = {
	async listProjects(signal?: AbortSignal): Promise<ProjectDto[]> {
		const payload = await http.get<unknown>('/projects', withSignal(signal))
		return parseOrThrow(z.array(projectDtoSchema), payload, 'catalog.projects')
	},

	async createProject(body: CreateProjectPayload): Promise<ProjectDto> {
		const payload = await http.post<unknown>('/projects', { body })
		return parseOrThrow(projectDtoSchema, payload, 'catalog.project')
	},

	async renameProject(project: string, body: RenameProjectPayload): Promise<ProjectDto> {
		const payload = await http.patch<unknown>(projectPath(project), { body })
		return parseOrThrow(projectDtoSchema, payload, 'catalog.project')
	},

	async deleteProject(project: string): Promise<void> {
		await http.delete<unknown>(projectPath(project))
	},

	async listStages(project: string, signal?: AbortSignal): Promise<StageDto[]> {
		const payload = await http.get<unknown>(
			`${projectPath(project)}/stages`,
			withSignal(signal),
		)
		return parseOrThrow(z.array(stageDtoSchema), payload, 'catalog.stages')
	},

	async createStage(project: string, body: CreateStagePayload): Promise<StageDto> {
		const payload = await http.post<unknown>(`${projectPath(project)}/stages`, { body })
		return parseOrThrow(stageDtoSchema, payload, 'catalog.stage')
	},

	async deleteStage(project: string, stage: string): Promise<void> {
		await http.delete<unknown>(stagePath(project, stage))
	},

	async listApps(project: string, stage: string, signal?: AbortSignal): Promise<AppDto[]> {
		const payload = await http.get<unknown>(
			`${stagePath(project, stage)}/apps`,
			withSignal(signal),
		)
		return parseOrThrow(z.array(appDtoSchema), payload, 'catalog.apps')
	},

	async getApp(
		project: string,
		stage: string,
		app: string,
		signal?: AbortSignal,
	): Promise<AppDto> {
		const payload = await http.get<unknown>(appPath(project, stage, app), withSignal(signal))
		return parseOrThrow(appDtoSchema, payload, 'catalog.app')
	},

	async createApp(project: string, stage: string, body: CreateAppPayload): Promise<AppDto> {
		const payload = await http.post<unknown>(`${stagePath(project, stage)}/apps`, { body })
		return parseOrThrow(appDtoSchema, payload, 'catalog.app')
	},

	async updateApp(
		project: string,
		stage: string,
		app: string,
		body: UpdateAppPayload,
	): Promise<AppDto> {
		const payload = await http.patch<unknown>(appPath(project, stage, app), { body })
		return parseOrThrow(appDtoSchema, payload, 'catalog.app')
	},

	async deleteApp(project: string, stage: string, app: string): Promise<void> {
		await http.delete<unknown>(appPath(project, stage, app))
	},
}
