<script lang="ts">
	import { resolve } from '$app/paths'
	import { page } from '$app/state'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { AuthLayout, Banner, LinkButton } from '@/shared/ui'
	import { AuthHeading } from '../../components/authHeading'
	import { AccountForm } from '../../features/accountForm'
	import { SetupViewModel } from './Setup.viewModel.svelte.ts'
	import { screen } from './styles.css'

	let { token }: { token: string | null } = $props()

	const vm = new SetupViewModel(() => token)
	const titleId = $props.id()
</script>

<AuthLayout appName="muelle" host={page.url.host}>
	<div class={screen}>
		<AuthHeading id={titleId} title={m.auth_setup_title()} subtitle={m.auth_setup_subtitle()} />

		{#if !vm.hasToken}
			<Banner tone="error" title={m.auth_setup_missing_token()} />
		{:else if vm.alreadySetUp}
			<Banner tone="error" title={m.auth_setup_already()}>
				{#snippet action()}
					<LinkButton href={resolve('/login')} variant="secondary" size="sm">
						{m.auth_go_login()}
					</LinkButton>
				{/snippet}
			</Banner>
		{:else if vm.formError}
			<Banner tone="error" title={vm.formError} />
		{/if}

		<AccountForm
			labelledBy={titleId}
			submitLabel={m.auth_setup_submit()}
			disabled={!vm.canSubmit || vm.isPending}
			loading={vm.isPending}
			serverErrors={vm.serverErrors}
			onSubmit={vm.submit}
		/>
	</div>
</AuthLayout>
