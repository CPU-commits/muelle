<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import {
		Badge,
		EmptyState,
		formatDateTime,
		formatNumber,
		formatRelative,
		List,
		ListRow,
		Menu,
		SearchInput,
	} from '@/shared/ui'
	import { EnvValueText } from '../../components/envValueText'
	import type { EnvEditorViewModel } from './EnvEditor.viewModel.svelte.ts'
	import {
		keyCell,
		listHeader,
		listHeading,
		listMeta,
		listSection,
		listTitle,
		noResults,
		rowActions,
		searchBox,
		updatedCell,
		valueCell,
	} from './styles.css'

	// Las variables propias del dueño, editables. En la app, las que también tiene el stage
	// llevan "pisa al stage": la de la app gana.
	type Props = { vm: EnvEditorViewModel; title: string; emptyTitle: string; emptyBody: string }

	let { vm, title, emptyTitle, emptyBody }: Props = $props()

	const id = $props.id()
</script>

<section class={listSection} aria-labelledby={`${id}-title`}>
	<div class={listHeader}>
		<div class={listHeading}>
			<h2 id={`${id}-title`} class={listTitle}>{title}</h2>
			<span class={listMeta}>{formatNumber(vm.count)}</span>
		</div>
		{#if vm.count > 0}
			<SearchInput
				class={searchBox}
				label={m.secrets_search_label()}
				inputSize="sm"
				placeholder={m.secrets_search_placeholder()}
				bind:value={vm.search}
			/>
		{/if}
	</div>

	{#if vm.count === 0}
		<EmptyState icon="lock" title={emptyTitle} body={emptyBody} />
	{:else if vm.visible.length === 0}
		<p class={noResults} role="status">{m.secrets_search_empty({ query: vm.search.trim() })}</p>
	{:else}
		<List label={title}>
			{#each vm.visible as envVar (envVar.key)}
				<ListRow>
					<span class={keyCell}>{envVar.key}</span>
					<span class={valueCell}>
						<EnvValueText {envVar} />
						{#if vm.overridden.has(envVar.key)}
							<Badge shape="pill">{m.secrets_overrides_stage()}</Badge>
						{/if}
					</span>
					<time
						class={updatedCell}
						datetime={envVar.updatedAt.toISOString()}
						title={m.secrets_updated({ date: formatDateTime(envVar.updatedAt) })}
					>
						{formatRelative(envVar.updatedAt)}
					</time>
					<span class={rowActions}>
						<Menu
							label={m.secrets_row_options({ key: envVar.key })}
							items={vm.menuFor(envVar)}
						/>
					</span>
				</ListRow>
			{/each}
		</List>
	{/if}
</section>
