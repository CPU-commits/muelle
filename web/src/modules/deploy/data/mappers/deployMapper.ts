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
	}
}

export function serviceStatusFromDto(dto: ServiceStatusDto): ServiceStatus {
	return {
		name: dto.name,
		desiredReplicas: dto.replicas_desired,
		runningReplicas: dto.replicas_running,
		cpuPercent: dto.cpu_percent ?? null,
		memoryBytes: dto.memory_bytes ?? null,
		memoryLimitBytes: dto.memory_limit_bytes ?? null,
		tasks: dto.tasks.map(taskFromDto),
	}
}

export function appStatusFromDto(dto: AppStatusDto): AppStatus {
	return { services: dto.services.map(serviceStatusFromDto) }
}

export function deployFromDto(dto: DeployDto): Deploy {
	return {
		id: asDeployId(dto.id),
		status: dto.status,
		images: Object.entries(dto.images)
			.map(([service, raw]) => ({ service, ...splitImageRef(raw) }))
			.sort((a, b) => a.service.localeCompare(b.service)),
		trigger: dto.trigger,
		actor: { type: dto.actor_type, name: dto.actor_name || null },
		error: dto.error || null,
		createdAt: new Date(dto.created_at),
		startedAt: dateOrNull(dto.started_at),
		finishedAt: dateOrNull(dto.finished_at),
	}
}

export function deployAcceptedFromDto(dto: DeployAcceptedDto): DeployAccepted {
	return { id: asDeployId(dto.id) }
}

export function logPageFromDto(dto: LogPageDto): LogPage {
	return { chunk: dto.chunk, nextOffset: asLogOffset(dto.next_offset), status: dto.status }
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
