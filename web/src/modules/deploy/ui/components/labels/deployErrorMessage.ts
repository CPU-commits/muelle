import { m } from '@/shared/ui/i18n/paraglide/messages'
import { DomainError, pendingPhase, UnavailableError, ValidationError } from '@/shared/domain'
import { DeployNotFound, NoPreviousDeploy } from '../../../domain'

/** El mensaje de un error del módulo para un banner o un toast. */
export function deployErrorMessage(error: unknown): string {
	const phase = pendingPhase(error)
	if (phase !== null) return m.common_coming_title({ phase })
	if (error instanceof NoPreviousDeploy) return m.deploy_rollback_none_body()
	if (error instanceof DeployNotFound) return m.deploy_not_found()
	if (error instanceof UnavailableError) return m.common_unavailable()
	if (error instanceof ValidationError) return error.message
	if (error instanceof DomainError && error.code === 'rate_limited') return error.message
	return m.common_error_generic()
}
