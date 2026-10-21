import {
	type AppLogLine,
	type AppStatus,
	asDeployId,
	asLogOffset,
	type Deploy,
	type DeployAccepted,
	type DeployRequest,
	type LogPage,
	parseAppLogText,
	type ServiceImage,
	type ServiceStatus,
	type ServiceTask,
	splitImageRef,
} from '../../domain'
import type {
	AppLogLineDto,
	AppStatusDto,
	CreateDeployPayload,
	DeployAcceptedDto,
	DeployDto,
	LogPageDto,
	ServiceStatusDto,
	TaskDto,
} from '../dtos/deployDto'

function dateOrNull(raw: string | null | undefined): Date | null {
	return raw ? new Date(raw) : null
}

export function taskFromDto(dto: TaskDto): ServiceTask {
	return {
		id: dto.id,
		slot: dto.slot ?? null,
		state: dto.state,
		desiredState: dto.desired_state,
		health: dto.health ?? null,
		node: dto.node ?? null,
		since: dateOrNull(dto.since),
		error: dto.error || null,
		cpuPercent: dto.cpu_percent ?? null,
		memoryBytes: dto.memory_bytes ?? null,
	}
}

export function serviceStatusFromDto(dto: ServiceStatusDto): ServiceStatus {
	return {
		name: dto.name,
		image: dto.image || null,
		updateState: dto.update_state || null,
		desiredReplicas: dto.replicas_desired,
		runningReplicas: dto.replicas_running,
		memoryLimitBytes: dto.memory_limit_bytes ?? null,
		tasks: dto.tasks.map(taskFromDto),
	}
}

export function appStatusFromDto(dto: AppStatusDto): AppStatus {
	return { services: dto.services.map(serviceStatusFromDto) }
}

/** `servicio → ref` a una lista por servicio, con el digest aparte. */
function serviceImages(images: Record<string, string>): ServiceImage[] {
	return Object.entries(images)
		.map(([service, raw]) => ({ service, ...splitImageRef(raw) }))
		.sort((a, b) => a.service.localeCompare(b.service))
}

export function deployFromDto(dto: DeployDto): Deploy {
	return {
		id: asDeployId(dto.id),
		status: dto.status,
		images: serviceImages(dto.images),
		requestedImages: dto.requested_images ? serviceImages(dto.requested_images) : null,
		trigger: dto.trigger,
		actor: { type: dto.actor_type, name: dto.actor_name || null },
		error: dto.error || null,
		createdAt: new Date(dto.created_at),
		startedAt: dateOrNull(dto.started_at),
		finishedAt: dateOrNull(dto.finished_at),
		supersededBy: dto.superseded_by ? asDeployId(dto.superseded_by) : null,
		routesWarning: dto.routes_warning ?? false,
	}
}

export function deployAcceptedFromDto(dto: DeployAcceptedDto): DeployAccepted {
	return { id: asDeployId(dto.id) }
}

export function logPageFromDto(dto: LogPageDto): LogPage {
	return {
		chunk: dto.chunk,
		nextOffset: asLogOffset(dto.next_offset),
		status: dto.status,
		supersededBy: dto.superseded_by ? asDeployId(dto.superseded_by) : null,
	}
}

export function appLogLineFromDto(dto: AppLogLineDto): AppLogLine {
	const { level, text } = parseAppLogText(dto.line)
	return {
		time: dateOrNull(dto.time),
		task: dto.task || null,
		stream: dto.stream,
		text,
		level,
	}
}

export function createDeployPayload(request: DeployRequest): CreateDeployPayload {
	switch (request.kind) {
		case 'redeploy':
			return {}
		case 'image':
			return { image: request.image }
		case 'images':
			return { images: { ...request.images } }
	}
}
