import { m } from '@/shared/ui/i18n/paraglide/messages'
import { ValidationError } from '@/shared/domain'
import {
	appSlugProblem,
	InvalidProjectName,
	InvalidSlug,
	isValidImage,
	PROJECT_SLUG_MAX,
	projectNameProblem,
	projectSlugProblem,
	SlugTaken,
	STAGE_SLUG_MAX,
	stageSlugProblem,
	APP_SLUG_MAX,
	normalizeSlug,
	composeProblem,
} from '../../../domain'
import {
	catalogErrorMessage,
	projectNameMessage,
	slugMessage,
	slugTakenMessage,
	specMessages,
} from '../../components/catalogMessages'

// Validación de UX de los diálogos de Proyectos, con mensajes traducidos. La regla de
// verdad vive en el dominio (el caso de uso vuelve a validar) y en el backend.

export function validateProjectName(value: string): string | undefined {
	const problem = projectNameProblem(value)
	return problem ? projectNameMessage(new InvalidProjectName(problem)) : undefined
}

function slugError(problem: 'format' | 'reserved' | null, max: number, value: string) {
	if (value.trim() === '') return m.common_required()
	return problem ? slugMessage(new InvalidSlug(problem, max), normalizeSlug(value)) : undefined
}

export function validateProjectSlug(value: string): string | undefined {
	return slugError(projectSlugProblem(value), PROJECT_SLUG_MAX, value)
}

export function validateStageSlug(value: string): string | undefined {
	return slugError(stageSlugProblem(value), STAGE_SLUG_MAX, value)
}

export function validateAppSlug(value: string): string | undefined {
	return slugError(appSlugProblem(value), APP_SLUG_MAX, value)
}

export function validateImage(value: string): string | undefined {
	const image = value.trim()
	if (image === '') return specMessages.imageRequired()
	return isValidImage(image) ? undefined : specMessages.imageInvalid()
}

export function validatePort(value: string): string | undefined {
	const port = Number(value)
	return value.trim() !== '' && Number.isInteger(port) && port >= 1 && port <= 65535
		? undefined
		: specMessages.port()
}

export function validateComposeYaml(value: string): string | undefined {
	const problem = composeProblem(value)
	if (problem === 'empty') return m.catalog_error_compose_empty()
	return problem === 'too_large' ? m.catalog_error_compose_too_large() : undefined
}

export interface DialogErrors {
	/** Por campo del formulario. */
	readonly fields: Readonly<Record<string, string>>
	/** Lo que no es de un campo: va en un aviso arriba del formulario. */
	readonly form: string | null
}

export const NO_DIALOG_ERRORS: DialogErrors = { fields: {}, form: null }

/** Al editar un campo, su error del backend ya no aplica. */
export function withoutField(errors: DialogErrors, field: string): DialogErrors {
	if (!(field in errors.fields)) return errors
	const fields = Object.fromEntries(
		Object.entries(errors.fields).filter(([name]) => name !== field),
	)
	return { ...errors, fields }
}

/**
 * Un error al crear o renombrar, puesto donde corresponde: el slug repetido y los 422 de
 * un campo que el formulario tiene, en su campo; lo demás, arriba.
 */
export function dialogErrors(error: unknown, formFields: readonly string[]): DialogErrors {
	if (error instanceof SlugTaken && formFields.includes('slug')) {
		return { fields: { slug: slugTakenMessage(error) }, form: null }
	}
	if (error instanceof ValidationError && error.field && formFields.includes(error.field)) {
		const message =
			error instanceof InvalidSlug || error instanceof InvalidProjectName
				? catalogErrorMessage(error)
				: error.message
		return { fields: { [error.field]: message }, form: null }
	}
	return { fields: {}, form: catalogErrorMessage(error) }
}
