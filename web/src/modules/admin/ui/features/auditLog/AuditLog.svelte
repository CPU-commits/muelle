<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import {
		Badge,
		Button,
		cellMono,
		EmptyState,
		ErrorState,
		Icon,
		LoadingState,
		LoadMore,
		Mono,
		Table,
	} from '@/shared/ui'
	import { SectionHeading } from '../../components/sectionHeading'
	import { AuditLogViewModel } from './AuditLog.viewModel.svelte.ts'
	import { moreError, section, target, via, when } from './styles.css'

	// La sección "Auditoría" de Ajustes (`Ajustes.dc.html`). `id` es el ancla de la nav.
	let { id }: { id?: string } = $props()

	const vm = new AuditLogViewModel()
	const titleId = $props.id()
</script>

<section {id} class={section} aria-labelledby={titleId}>
	<SectionHeading
		id={titleId}
		title={m.admin_audit_title()}
		description={m.admin_audit_description()}
	>
		{#snippet actions()}
			<Button
				variant="ghost"
				size="sm"
				loading={vm.isRefreshing}
				disabled={vm.isLoading}
				onclick={vm.refresh}
			>
				<Icon name="refresh" size={14} />
				{m.admin_audit_refresh()}
			</Button>
		{/snippet}
	</SectionHeading>

	{#if vm.isLoading}
		<LoadingState label={m.common_loading()} />
	{:else if vm.loadError}
		<ErrorState title={vm.loadError} retryLabel={m.common_retry()} onRetry={vm.refresh} />
	{:else if vm.isEmpty}
		<EmptyState title={m.admin_audit_empty_title()} body={m.admin_audit_empty_body()} />
	{:else}
		<Table minWidth="600px" caption={m.admin_audit_caption()}>
			<thead>
				<tr>
					<th scope="col">{m.admin_audit_when()}</th>
					<th scope="col">{m.admin_audit_who()}</th>
					<th scope="col">{m.admin_audit_action()}</th>
					<th scope="col">{m.admin_audit_target()}</th>
				</tr>
			</thead>
			<tbody>
				{#each vm.rows as row (row.id)}
					<tr>
						<td class={when}><time datetime={row.iso}>{row.when}</time></td>
						<td>
							{#if row.actor.kind === 'token'}
								{m.admin_audit_actor_token()} <Mono>{row.actor.name}</Mono>
							{:else}
								{row.actor.text}
							{/if}
							{#if row.viaMcp}
								<span class={via}><Badge>{m.admin_audit_via_mcp()}</Badge></span>
							{/if}
						</td>
						<td class={cellMono}>{row.action}</td>
						<td class={[cellMono, target]}>{row.target}</td>
					</tr>
				{/each}
			</tbody>
		</Table>
		{#if vm.moreError}
			<p class={moreError} role="alert">{vm.moreError}</p>
		{/if}
		<LoadMore
			hasMore={vm.hasMore}
			loading={vm.isLoadingMore}
			onLoadMore={vm.loadMore}
			label={vm.moreError ? m.common_retry() : m.admin_audit_more()}
		/>
	{/if}
</section>
