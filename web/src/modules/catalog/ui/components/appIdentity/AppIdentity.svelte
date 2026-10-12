<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { Mono } from '@/shared/ui'
	import type { AppSummary } from '../../../domain'
	import { kindLabel } from '../catalogMessages'
	import { details, item } from './styles.css'

	// Qué es la app y dónde vive en Swarm, para el encabezado de sus pantallas:
	// "Servicio · stack auditfy_staging_api · red auditfy_staging · imagen ghcr.io/…".
	// Va en el `details` de un <PageHeader>; los datos salen de `appSummaryQuery`.
	let { summary }: { summary: AppSummary } = $props()
</script>

<p class={details}>
	<span>{kindLabel(summary.kind)}</span>
	<span class={item}>
		{m.catalog_identity_stack()}
		<Mono tone="secondary">{summary.stackName}</Mono>
	</span>
	<span class={item}>
		{m.catalog_identity_network()}
		<Mono tone="secondary">{summary.stageNetwork}</Mono>
	</span>
	{#if summary.contents.kind === 'image'}
		<span class={item}>
			{m.catalog_identity_image()}
			<Mono tone="secondary" truncate title={summary.contents.image}>
				{summary.contents.image}
			</Mono>
		</span>
	{:else if summary.contents.services.length > 0}
		<span class={item}>
			{m.catalog_identity_services()}
			<Mono tone="secondary">{summary.contents.services.join(' · ')}</Mono>
		</span>
	{/if}
</p>
