<script lang="ts">
	import type { Snippet } from 'svelte'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { PageHeader } from '@/shared/ui'
	import type { AppSummary } from '../../../domain'
	import { AppChangesBadge, AppIdentity } from '../appIdentity'
	import { details as detailsClass, subtitle as subtitleClass } from './styles.css'

	// El h1 "Configuración" de una app, con el estado del deploy, la explicación y la
	// línea de identidad (tipo, stack, red, imagen).
	type Props = { summary: AppSummary; subtitle: Snippet; actions?: Snippet }

	let { summary, subtitle, actions }: Props = $props()
</script>

<PageHeader title={m.nav_app_config()} {...actions ? { actions } : {}}>
	{#snippet badges()}
		<AppChangesBadge state={summary.deployState} />
	{/snippet}
	{#snippet details()}
		<div class={detailsClass}>
			<p class={subtitleClass}>{@render subtitle()}</p>
			<AppIdentity {summary} />
		</div>
	{/snippet}
</PageHeader>
