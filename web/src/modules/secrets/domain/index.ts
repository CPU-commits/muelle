export type { EnvOwner, EnvOwnerKind } from './entities/envOwner'
export { appOwner, ownerLabel, ownerStage, sameOwner, stageOwner } from './entities/envOwner'
export type {
	EnvPatch,
	EnvPatchInput,
	EnvReplacement,
	EnvReplacementInput,
	RawEnvEntries,
} from './entities/envPatch'
export {
	assertPatchWithinLimits,
	parseEnvPatch,
	parseEnvReplacement,
	patchLimitProblem,
} from './entities/envPatch'
export type { EnvVar } from './entities/envVar'
export {
	compareEnvKeys,
	envKeys,
	filterEnvVars,
	findEnvVar,
	knownEnvSize,
	overlappingKeys,
	sortEnvVars,
} from './entities/envVar'
export type { EnvField, EnvKeyProblem, EnvLimit, EnvValueProblem } from './errors/secretsErrors'
export {
	EmptyEnvPatch,
	EnvKeyInSetAndUnset,
	EnvLimitExceeded,
	EnvOutOfScope,
	EnvOwnerNotFound,
	InvalidEnvKey,
	InvalidEnvValue,
} from './errors/secretsErrors'
export type { EnvRepository, SecretsDeps } from './repositories/envRepository'
export type {
	DotenvDuplicate,
	DotenvEntry,
	DotenvIssue,
	DotenvProblem,
	ParsedDotenv,
} from './services/dotenv'
export { parseDotenv } from './services/dotenv'
export type {
	ImportChange,
	ImportChangeReason,
	ImportEntry,
	ImportMode,
	ImportOptions,
	ImportPlan,
} from './services/importPlan'
export { planImport } from './services/importPlan'
export type { GetEnv } from './use-cases/getEnv'
export { makeGetEnv } from './use-cases/getEnv'
export type { PatchEnv, PatchEnvInput } from './use-cases/patchEnv'
export { makePatchEnv } from './use-cases/patchEnv'
export type { ReplaceEnv, ReplaceEnvInput } from './use-cases/replaceEnv'
export { makeReplaceEnv } from './use-cases/replaceEnv'
export type { EnvKey } from './value-objects/envKey'
export {
	ENV_KEY_MAX_LENGTH,
	ENV_KEY_PATTERN,
	envKeyProblem,
	isValidEnvKey,
	normalizeEnvKey,
	parseEnvKey,
	RESERVED_ENV_PREFIX,
} from './value-objects/envKey'
export {
	envValueProblem,
	envVarSize,
	MAX_ENV_VARS,
	MAX_OWNER_BYTES,
	MAX_VALUE_BYTES,
	utf8Length,
} from './value-objects/envLimits'
