<script lang="ts">
	import { page } from '$app/state'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { AuthLayout, Banner } from '@/shared/ui'
	import { AuthHeading } from '../../components/authHeading'
	import { AccountForm } from '../../features/accountForm'
	import { InviteViewModel } from './Invite.viewModel.svelte.ts'
	import { screen } from './styles.css'

	let { token }: { token: string } = $props()

	const vm = new InviteViewModel(() => token)
	const titleId = $props.id()
</script>

<AuthLayout appName="muelle" host={page.url.host}>
	<div class={screen}>
		<AuthHeading
			id={titleId}
			title={m.auth_invite_title()}
			subtitle={m.auth_invite_subtitle()}
		/>

		{#if vm.formError}
			<Banner tone="error" title={vm.formError} />
		{/if}

		<AccountForm
			labelledBy={titleId}
			submitLabel={m.auth_invite_submit()}
			disabled={vm.invalid || vm.isPending}
			loading={vm.isPending}
			serverErrors={vm.serverErrors}
			onSubmit={vm.submit}
		/>
	</div>
</AuthLayout>
