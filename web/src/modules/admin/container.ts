import {
	httpAuditRepository,
	httpRegistryRepository,
	httpSettingsRepository,
} from './data/repositories/httpAdminRepositories'
import {
	makeAddRegistry,
	makeGetSettings,
	makeListAudit,
	makeListRegistries,
	makeRemoveCloudflareToken,
	makeRemoveRegistry,
	makeReplaceCloudflareToken,
	makeSetCloudflareOriginCa,
	makeUpdateAcmeEmail,
	makeUpdateProxyPorts,
	makeUpdatePublicIp,
	makeUpdateRegistry,
	makeUpdateSecurity,
} from './domain'

// Composition root del módulo: el ÚNICO lugar que conoce los repositorios concretos.
const settings = { settings: httpSettingsRepository }
const audit = { audit: httpAuditRepository }
const registries = { registries: httpRegistryRepository }

export const adminUseCases = {
	getSettings: makeGetSettings(settings),
	updateSecurity: makeUpdateSecurity(settings),
	updatePublicIp: makeUpdatePublicIp(settings),
	updateAcmeEmail: makeUpdateAcmeEmail(settings),
	updateProxyPorts: makeUpdateProxyPorts(settings),
	replaceCloudflareToken: makeReplaceCloudflareToken(settings),
	removeCloudflareToken: makeRemoveCloudflareToken(settings),
	setCloudflareOriginCa: makeSetCloudflareOriginCa(settings),
	listAudit: makeListAudit(audit),
	listRegistries: makeListRegistries(registries),
	addRegistry: makeAddRegistry(registries),
	updateRegistry: makeUpdateRegistry(registries),
	removeRegistry: makeRemoveRegistry(registries),
}
