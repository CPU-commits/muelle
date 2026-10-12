import { ConflictError, NotFoundError, ValidationError } from '@/shared/domain'

// Un error por cada caso del catálogo que el panel distingue (los `error.code` de
// catalog/infra/in/http/mapper_errors.go y los comunes de cmd/http). Los mensajes son para
// logs: la pantalla elige su texto traducido por la clase.

/** 404 de un proyecto. */
export class ProjectNotFound extends NotFoundError {
	constructor(options?: ErrorOptions) {
		super('El proyecto no existe', options)
		this.name = 'ProjectNotFound'
	}
}

/** 404 de un stage (o de su proyecto). */
export class StageNotFound extends NotFoundError {
	constructor(options?: ErrorOptions) {
		super('El stage no existe', options)
		this.name = 'StageNotFound'
	}
}

/** 404 de una app (o de su stage o proyecto). */
export class AppNotFound extends NotFoundError {
	constructor(options?: ErrorOptions) {
		super('La app no existe', options)
		this.name = 'AppNotFound'
	}
}

/** `not_empty` al borrar un proyecto: todavía tiene stages. */
export class ProjectNotEmpty extends ConflictError {
	constructor(options?: ErrorOptions) {
		super('El proyecto todavía tiene stages', options)
		this.name = 'ProjectNotEmpty'
	}
}

/** `not_empty` al borrar un stage: todavía tiene apps. */
export class StageNotEmpty extends ConflictError {
	constructor(options?: ErrorOptions) {
		super('El stage todavía tiene apps', options)
		this.name = 'StageNotEmpty'
	}
}

/** `conflict` al crear: el slug ya está usado (en el panel, o en su proyecto o stage). */
export class SlugTaken extends ConflictError {
	readonly entity: 'project' | 'stage' | 'app'

	constructor(entity: 'project' | 'stage' | 'app', options?: ErrorOptions) {
		super('Ese slug ya existe', options)
		this.name = 'SlugTaken'
		this.entity = entity
	}
}

/**
 * `kind_mismatch`: se mandó el spec de una app compose o el compose de una de formulario.
 * El panel no debería provocarlo; si pasa, la app cambió de tipo en otra pestaña.
 */
export class KindMismatch extends ValidationError {
	constructor(options?: ErrorOptions) {
		super('El campo no corresponde al tipo de app', 'kind', options)
		this.name = 'KindMismatch'
	}
}

export type SlugProblem = 'format' | 'reserved'

/** Un slug fuera de la regla del backend (validate.ProjectSlug, StageSlug, AppSlug). */
export class InvalidSlug extends ValidationError {
	readonly problem: SlugProblem
	readonly max: number

	constructor(problem: SlugProblem, max: number) {
		super(`Slug inválido (${problem})`, 'slug')
		this.name = 'InvalidSlug'
		this.problem = problem
		this.max = max
	}
}

export type ProjectNameProblem = 'empty' | 'long' | 'invisible'

export class InvalidProjectName extends ValidationError {
	readonly problem: ProjectNameProblem

	constructor(problem: ProjectNameProblem) {
		super(`Nombre inválido (${problem})`, 'name')
		this.name = 'InvalidProjectName'
		this.problem = problem
	}
}

/**
 * El spec de una app de formulario no pasa las reglas de `vo.AppSpec` del backend.
 * `field` usa los nombres del dominio: `port`, `healthcheck.intervalSeconds`,
 * `volumes[0].mount`, `resources.memoryMb`.
 */
export class InvalidAppSpec extends ValidationError {
	constructor(field: string, message: string) {
		super(message, field)
		this.name = 'InvalidAppSpec'
	}
}

export type ComposeProblem = 'empty' | 'too_large'

export class InvalidCompose extends ValidationError {
	readonly problem: ComposeProblem

	constructor(problem: ComposeProblem) {
		super(
			problem === 'empty' ? 'El compose es obligatorio' : 'El compose supera 256 KiB',
			'composeYaml',
		)
		this.name = 'InvalidCompose'
		this.problem = problem
	}
}

export class InvalidDeployTimeout extends ValidationError {
	constructor() {
		super('El timeout del deploy debe estar entre 30 y 3600 segundos', 'deployTimeoutSeconds')
		this.name = 'InvalidDeployTimeout'
	}
}
