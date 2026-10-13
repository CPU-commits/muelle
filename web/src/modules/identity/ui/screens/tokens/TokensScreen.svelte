<script lang="ts">
	import type { Attachment } from 'svelte/attachments'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import {
		Badge,
		Button,
		Card,
		ConfirmDialog,
		CopyButton,
		EmptyState,
		ErrorState,
		Icon,
		Input,
		List,
		ListRow,
		LoadingState,
		Mono,
	} from '@/shared/ui'
	import { CreateTokenModal } from '../../features/createTokenModal'
	import {
		copyInput,
		copyRow,
		createdText,
		createdTitle,
		creator,
		expiredText,
		group,
		groupCount,
		groupHeading,
		groupTitle,
		header,
		headerText,
		metaCell,
		nameCell,
		nameLine,
		newBadge,
		scopeCell,
		screen,
		snippet,
		subtitle,
		title,
		tokenName,
	} from './styles.css'
	import { type TokenRow, TokensViewModel } from './Tokens.viewModel.svelte.ts'

	const vm = new TokensViewModel()
	const activeId = $props.id()
	const expiredId = `${activeId}-expired`

	// El token recién creado queda seleccionado, listo para copiar a mano si hace falta.
	const selectOnMount: Attachment<HTMLInputElement> = (input) => {
		input.focus()
		input.select()
	}
</script>

{#snippet tokenRow(row: TokenRow, expired: boolean)}
	<ListRow height="lg" muted={expired}>
		<span class={nameCell}>
			<span class={nameLine}>
				<span class={tokenName}><Mono>{row.token.name}</Mono></span>
				{#if row.isNew}
					<span class={newBadge}>
						<Badge tone="strong" shape="pill">{m.identity_tokens_new()}</Badge>
					</span>
				{/if}
			</span>
			{#if row.creator}
				<span class={creator}>{row.creator}</span>
			{/if}
		</span>
		<span class={scopeCell}><Mono size={13} tone="secondary">{row.scope}</Mono></span>
		<span class={metaCell}>{row.lastUsed}</span>
		<span class={[metaCell, expired && expiredText]}>{row.expiry}</span>
		<Button
			variant="ghost"
			size="sm"
			aria-label={m.identity_tokens_revoke_label({ name: row.token.name })}
			onclick={() => vm.askRevoke(row.token)}
		>
			{m.identity_tokens_revoke()}
		</Button>
	</ListRow>
{/snippet}

<div class={screen}>
	<div class={header}>
		<div class={headerText}>
			<h2 class={title}>{m.identity_tokens_title()}</h2>
			<p class={subtitle}>{m.identity_tokens_subtitle()}</p>
		</div>
		{#if !vm.isEmpty}
			<Button size="sm" onclick={vm.openCreate}>
				<Icon name="plus" size={14} />
				{m.identity_tokens_create()}
			</Button>
		{/if}
	</div>

	{#if vm.created}
		<Card tone="highlight" note={m.identity_tokens_once()}>
			<!-- El título va acá y no en `title` de Card: el nombre del token va en mono. -->
			<h3 class={createdTitle}>
				{m.identity_tokens_created_title()}
				<Mono size={14} tone="primary">{vm.created.token.name}</Mono>
			</h3>
			<p class={createdText}>
				{m.identity_tokens_created_body()}
				<Mono size={13} tone="primary">MUELLE_TOKEN</Mono>.
			</p>
			<div class={copyRow}>
				<Input
					class={copyInput}
					mono
					readonly
					aria-label={m.identity_tokens_secret_label()}
					value={vm.created.secret}
					onfocus={(event) => event.currentTarget.select()}
					{@attach selectOnMount}
				/>
				<CopyButton value={vm.created.secret} />
			</div>
			<pre class={snippet} aria-label={m.identity_tokens_snippet_label()}>{vm.snippet}</pre>
			{#snippet footer()}
				<Button size="sm" onclick={vm.dismissCreated}>{m.identity_tokens_done()}</Button>
			{/snippet}
		</Card>
	{/if}

	{#if vm.isLoading}
		<LoadingState label={m.common_loading()} />
	{:else if vm.loadError}
		<ErrorState title={vm.loadError} retryLabel={m.common_retry()} onRetry={vm.retry} />
	{:else if vm.isEmpty}
		<EmptyState
			icon="lock"
			title={m.identity_tokens_empty_title()}
			body={m.identity_tokens_empty_body()}
		>
			{#snippet action()}
				<Button size="sm" onclick={vm.openCreate}>
					<Icon name="plus" size={14} />
					{m.identity_tokens_create()}
				</Button>
			{/snippet}
		</EmptyState>
	{:else}
		{#if vm.activeRows.length > 0}
			<section class={group} aria-labelledby={activeId}>
				<div class={groupHeading}>
					<h3 id={activeId} class={groupTitle}>{m.identity_tokens_active()}</h3>
					<span class={groupCount}>{vm.activeRows.length}</span>
				</div>
				<List label={m.identity_tokens_list_active()}>
					{#each vm.activeRows as row (row.token.id)}
						{@render tokenRow(row, false)}
					{/each}
				</List>
			</section>
		{/if}
		{#if vm.expiredRows.length > 0}
			<section class={group} aria-labelledby={expiredId}>
				<div class={groupHeading}>
					<h3 id={expiredId} class={groupTitle}>{m.identity_tokens_expired()}</h3>
					<span class={groupCount}>{vm.expiredRows.length}</span>
				</div>
				<List label={m.identity_tokens_list_expired()}>
					{#each vm.expiredRows as row (row.token.id)}
						{@render tokenRow(row, true)}
					{/each}
				</List>
			</section>
		{/if}
	{/if}
</div>

{#if vm.createOpen}
	<CreateTokenModal onCreated={vm.onCreated} onClose={vm.closeCreate} />
{/if}

<ConfirmDialog
	open={vm.revoking !== null}
	title={m.identity_tokens_revoke_title({ name: vm.revoking?.name ?? '' })}
	body={m.identity_tokens_revoke_body()}
	confirmLabel={m.identity_tokens_revoke()}
	cancelLabel={m.common_cancel()}
	loading={vm.isRevoking}
	onConfirm={vm.confirmRevoke}
	onCancel={vm.cancelRevoke}
/>
