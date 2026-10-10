import type { ApiError } from '@/shared/data/http/errors'
import { type DomainErrorMap, withDomainErrors } from '@/shared/data/http/domainErrors'
import type { AuthRepository } from '../../domain'
import {
	AccountLocked,
	AlreadySetUp,
	InvalidCode,
	InvalidCredentials,
	InvalidInvite,
	MfaRequired,
	NoMfaPending,
	NoPendingTotp,
	SessionExpired,
	TooManyAttempts,
	TotpSetupRequired,
	UsernameTaken,
} from '../../domain'
import { authHttpDataSource } from '../datasources/authHttpDataSource'
import {
	codePayload,
	credentialsPayload,
	loginOutcomeFromDto,
	meFromDto,
	setupPayload,
	totpSetupFromDto,
	usernameFromDto,
} from '../mappers/authMapper'

const cause = (error: ApiError) => ({ cause: error })

// Los `error.code` de identity (mapper_errors.go) y los de sesión de cmd/http.
const AUTH_ERRORS: DomainErrorMap = {
	codes: {
		invalid_credentials: (error) => new InvalidCredentials(cause(error)),
		locked: (error) => new AccountLocked(cause(error)),
		rate_limited: (error) => new TooManyAttempts(cause(error)),
		invalid_code: (error) => new InvalidCode(cause(error)),
		mfa_required: (error) => new MfaRequired(cause(error)),
		unauthenticated: (error) => new SessionExpired(cause(error)),
		totp_setup_required: (error) => new TotpSetupRequired(cause(error)),
		invalid_invite: (error) => new InvalidInvite(cause(error)),
		already_set_up: (error) => new AlreadySetUp(cause(error)),
		no_pending_totp: (error) => new NoPendingTotp(cause(error)),
		no_mfa_pending: (error) => new NoMfaPending(cause(error)),
	},
}

// Al crear una cuenta, un `conflict` es el nombre tomado.
const NEW_ACCOUNT_ERRORS: DomainErrorMap = {
	codes: {
		...AUTH_ERRORS.codes,
		conflict: (error) => new UsernameTaken(cause(error)),
	},
}

export const httpAuthRepository: AuthRepository = {
	me: (signal) =>
		withDomainErrors(async () => meFromDto(await authHttpDataSource.me(signal)), AUTH_ERRORS),

	login: (credentials) =>
		withDomainErrors(
			async () =>
				loginOutcomeFromDto(
					await authHttpDataSource.login(credentialsPayload(credentials)),
				),
			AUTH_ERRORS,
		),

	completeTotp: (code) =>
		withDomainErrors(async () => {
			await authHttpDataSource.completeTotp(codePayload(code))
		}, AUTH_ERRORS),

	logout: () => withDomainErrors(() => authHttpDataSource.logout(), AUTH_ERRORS),

	setup: (token, account) =>
		withDomainErrors(
			async () =>
				usernameFromDto(await authHttpDataSource.setup(setupPayload(token, account))),
			NEW_ACCOUNT_ERRORS,
		),

	acceptInvite: (token, account) =>
		withDomainErrors(
			async () =>
				usernameFromDto(
					await authHttpDataSource.acceptInvite(token, credentialsPayload(account)),
				),
			NEW_ACCOUNT_ERRORS,
		),

	beginTotpSetup: () =>
		withDomainErrors(
			async () => totpSetupFromDto(await authHttpDataSource.beginTotpSetup()),
			AUTH_ERRORS,
		),

	enableTotp: (code) =>
		withDomainErrors(() => authHttpDataSource.enableTotp(codePayload(code)), AUTH_ERRORS),
}
