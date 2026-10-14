<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { ErrorState, Icon, LinkButton, List, ListRow, LoadingState } from '@/shared/ui'
	import type { EnvVar } from '../../../domain'
	import { EnvValueText } from '../../components/envValueText'
	import {
		empty,
		header,
		heading,
		keyCell,
		keyReplaced,
		meta,
		noteCell,
		section,
		title,
		valueCell,
	} from './styles.css'

	// "Heredadas de <stage>": las variables del stage en solo lectura, con un link a su
	// página. Las que la app reemplaza van tachadas.
	type Props = {
		stage: string
		vars: readonly EnvVar[]
		/** Las claves que la app también define. */
		overridden: ReadonlySet<string>
		href: string
		loading: boolean
		error: string | null
		onRetry: () => void
	}

	let { stage, vars, overridden, href, loading, error, onRetry }: Props = $props()

	const id = $props.id()
</script>

<section class={section} aria-labelledby={`${id}-title`}>
	<div class={header}>
		<div class={heading}>
			<h2 id={`${id}-title`} class={title}>{m.secrets_inherited_title({ stage })}</h2>
			{#if !loading && !error}
				<span class={meta}>{m.secrets_inherited_meta({ count: vars.length })}</span>
			{/if}
		</div>
		<LinkButton {href} variant="ghost" size="sm">
			{m.secrets_inherited_edit()}
			<Icon name="arrowUpRight" size={14} />
		</LinkButton>
	</div>

	{#if loading}
		<LoadingState label={m.secrets_loading()} />
	{:else if error}
		<ErrorState
			title={m.secrets_inherited_error()}
			body={error}
			retryLabel={m.common_retry()}
			{onRetry}
		/>
	{:else if vars.length === 0}
		<p class={empty}>{m.secrets_inherited_empty()}</p>
	{:else}
		<List label={m.secrets_inherited_title({ stage })}>
			{#each vars as envVar (envVar.key)}
				{@const replaced = overridden.has(envVar.key)}
				<ListRow muted height="sm">
					<span class={[keyCell, replaced && keyReplaced]}>{envVar.key}</span>
					<span class={valueCell}>
						<EnvValueText {envVar} tone="tertiary" struck={replaced} labelSecret />
					</span>
					<span class={noteCell}>{replaced ? m.secrets_inherited_replaced() : ''}</span>
				</ListRow>
			{/each}
		</List>
	{/if}
</section>
