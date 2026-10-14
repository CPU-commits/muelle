// Superficie pública del módulo. Otros módulos y las rutas importan SOLO de acá.
export { adminUseCases } from './container'
export { adminKeys, auditQuery, settingsQuery } from './data'
export type { AuditEvent, Settings } from './domain'
export { SettingsLayout, SettingsScreen } from './ui'
