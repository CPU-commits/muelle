import { httpEnvRepository } from './data/repositories/httpEnvRepository'
import { makeGetEnv, makePatchEnv, makeReplaceEnv } from './domain'

// Composition root del módulo: el ÚNICO lugar que conoce el repositorio concreto.
const deps = { env: httpEnvRepository }

export const secretsUseCases = {
	getEnv: makeGetEnv(deps),
	replaceEnv: makeReplaceEnv(deps),
	patchEnv: makePatchEnv(deps),
}
