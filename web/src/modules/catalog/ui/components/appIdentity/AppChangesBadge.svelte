<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Badge } from '@/shared/ui'
	import type { DeployState } from '../../../domain'

	// Lo que el catálogo sabe del deploy (Fase 1): "Sin desplegar" o "Cambios sin
	// desplegar". Al día, no muestra nada; el estado real ("Corriendo") es de la Fase 2.
	let { state, size = 'md' }: { state: DeployState; size?: 'sm' | 'md' } = $props()
</script>

{#if state === 'pending_changes'}
	<Badge tone="warn" shape="pill" {size} dot>{m.catalog_badge_pending()}</Badge>
{:else if state === 'never_deployed'}
	<Badge shape="pill" {size} dot="idle">{m.catalog_badge_never()}</Badge>
{/if}
