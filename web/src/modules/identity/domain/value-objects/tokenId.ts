import type { Brand } from '@/shared/domain'

/** El id numérico de un token de API (`DELETE /tokens/{id}`). */
export type TokenId = Brand<number, 'TokenId'>
