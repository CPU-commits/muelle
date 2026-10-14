import { m } from '@/shared/ui/i18n/paraglide/messages'
import { describeError } from '@/shared/ui'
import { pendingPhase, ValidationError } from '@/shared/domain'
import { DeployNotFound, DockerUnavailable, NoPreviousDeploy } from '../../../domain'

/** El mensaje de un error del módulo para un banner o un toast. */
export function deployErrorMessage(error: unknown): string {
	const phase = pendingPhase(error)
	if (phase !== null) return m.common_coming_title({ phase })
	if (error instanceof NoPreviousDeploy) return m.deploy_rollback_none_body()
	if (error instanceof DeployNotFound) return m.deploy_not_found()
	if (error instanceof DockerUnavailable) return m.deploy_error_docker()
	if (error instanceof ValidationError) return error.message
	// El resto (sin red, un 5xx, un timeout, un 429…): el texto de siempre del panel.
	return describeError(error)
}
