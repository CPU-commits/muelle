<script lang="ts">
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import {
		Avatar,
		Button,
		ConfirmDialog,
		CopyButton,
		ErrorState,
		Field,
		Input,
		List,
		ListRow,
		LoadingState,
		Menu,
		Modal,
	} from '@/shared/ui'
	import {
		actionsCell,
		copyInput,
		copyRow,
		heading,
		headingText,
		inviteBody,
		name as nameClass,
		nameCell,
		note,
		section,
		sinceCell,
		subtitle,
		title,
		totpCell,
		you,
	} from './styles.css'
	import { UsersSectionViewModel } from './UsersSection.viewModel.svelte.ts'

	// La sección "Usuarios" de Ajustes (`Ajustes.dc.html`). `id` es el ancla de la nav.
	let { id }: { id?: string } = $props()

	const vm = new UsersSectionViewModel()
	const titleId = $props.id()
</script>

<section {id} class={section} aria-labelledby={titleId}>
	<div class={heading}>
		<div class={headingText}>
			<h2 id={titleId} class={title}>{m.identity_users_title()}</h2>
			<p class={subtitle}>{m.identity_users_subtitle()}</p>
		</div>
		<Button variant="secondary" size="sm" loading={vm.isInviting} onclick={vm.createInvite}>
			{m.identity_users_invite()}
		</Button>
	</div>

	{#if vm.isLoading}
		<LoadingState label={m.common_loading()} />
	{:else if vm.loadError}
		<ErrorState title={vm.loadError} retryLabel={m.common_retry()} onRetry={vm.retry} />
	{:else}
		<List label={m.identity_users_label()}>
			{#each vm.rows as row (row.user.username)}
				<ListRow height="lg">
					<span class={nameCell}>
						<Avatar name={row.user.username} size="sm" />
						<span class={nameClass}>{row.user.username}</span>
						{#if row.isYou}
							<span class={you}>{m.identity_users_you()}</span>
						{/if}
					</span>
					<span class={totpCell}>
						{row.user.totpEnabled
							? m.identity_users_totp_on()
							: m.identity_users_totp_off()}
					</span>
					<span class={sinceCell}>{row.since}</span>
					<span class={actionsCell}>
						{#if row.menu.length > 0}
							<Menu
								label={m.identity_users_options({ username: row.user.username })}
								items={row.menu}
							/>
						{/if}
					</span>
				</ListRow>
			{/each}
		</List>
	{/if}
</section>

<Modal
	open={vm.invite !== null}
	title={m.identity_invite_title()}
	description={m.identity_invite_body()}
	onClose={vm.keepInviteOpen}
>
	{#if vm.invite}
		<div class={inviteBody}>
			<Field label={m.identity_invite_url()}>
				{#snippet children(a11y)}
					<div class={copyRow}>
						<Input
							{...a11y}
							class={copyInput}
							mono
							readonly
							value={vm.invite?.url ?? ''}
							onfocus={(event) => event.currentTarget.select()}
						/>
						<CopyButton value={vm.invite?.url ?? ''} />
					</div>
				{/snippet}
			</Field>
			<p class={note}>{vm.inviteExpiry} {m.identity_invite_once()}</p>
		</div>
	{/if}
	{#snippet footer()}
		<Button size="sm" onclick={vm.closeInvite}>{m.identity_invite_done()}</Button>
	{/snippet}
</Modal>

<ConfirmDialog
	open={vm.pending !== null}
	title={vm.confirmTitle}
	body={vm.confirmBody}
	confirmLabel={vm.confirmLabel}
	cancelLabel={m.common_cancel()}
	loading={vm.isConfirming}
	onConfirm={vm.confirm}
	onCancel={vm.cancel}
/>
