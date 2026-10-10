import { httpAuthRepository } from './data/repositories/httpAuthRepository'
import {
	makeAcceptInvite,
	makeBeginTotpSetup,
	makeCompleteTotp,
	makeEnableTotp,
	makeGetMe,
	makeLogin,
	makeLogout,
	makeSetup,
} from './domain'

// Composition root del módulo: el ÚNICO lugar que conoce el repositorio concreto.
const deps = { auth: httpAuthRepository }

export const authUseCases = {
	getMe: makeGetMe(deps),
	login: makeLogin(deps),
	completeTotp: makeCompleteTotp(deps),
	logout: makeLogout(deps),
	setup: makeSetup(deps),
	acceptInvite: makeAcceptInvite(deps),
	beginTotpSetup: makeBeginTotpSetup(deps),
	enableTotp: makeEnableTotp(deps),
}
