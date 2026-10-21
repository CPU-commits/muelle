<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import type { RoadmapPhase } from '@/shared/domain'
	import EmptyState from '../emptyState/EmptyState.svelte'

	// Lo que se muestra cuando un endpoint de la Fase 2 o 3 todavía no existe en el backend:
	// el repositorio tradujo el 404 a un `UnavailableError` con `phase`, y la pantalla
	// pone esto en vez de un error. Con `pnpm dev:mock` los mocks responden y no aparece.
	let { phase, feature }: { phase: RoadmapPhase; feature?: string } = $props()
</script>

<EmptyState
	icon="clock"
	title={feature
		? m.common_coming_feature_title({ feature, phase })
		: m.common_coming_title({ phase })}
	body={m.common_coming_body()}
/>
