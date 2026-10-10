<script lang="ts">
	import { resolve } from '$app/paths'
	import { page } from '$app/state'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import {
		AuthLayout,
		Banner,
		Button,
		CodeInput,
		CopyButton,
		Field,
		LinkButton,
		LoadingState,
		Mono,
	} from '@/shared/ui'
	import type { Me } from '../../../domain'
	import { AuthHeading } from '../../components/authHeading'
	import { QrCode } from '../../components/qrCode'
	import { TotpSetupViewModel } from './TotpSetup.viewModel.svelte.ts'
	import {
		footer,
		form as formClass,
		qrBlock,
		screen,
		secret,
		secretLabel,
		secretRow,
	} from './styles.css'

	let { me }: { me: Me } = $props()

	const vm = new TotpSetupViewModel(() => me)
	const titleId = $props.id()
</script>

<AuthLayout appName="muelle" host={page.url.host} width="md">
	<div class={screen}>
		<AuthHeading id={titleId} title={m.auth_2fa_title()} subtitle={m.auth_2fa_subtitle()} />

		{#if vm.isRequired}
			<Banner tone="warn" title={m.auth_2fa_required()} />
		{/if}
		{#if vm.formError}
			<Banner tone="error" title={vm.formError} />
		{/if}

		{#if vm.setup}
			<div class={qrBlock}>
				<QrCode value={vm.setup.otpauthUrl} label={m.auth_2fa_qr_label()} />
				<div class={secret}>
					<span class={secretLabel}>{m.auth_2fa_secret()}</span>
					<div class={secretRow}>
						<Mono size={13}>{vm.secretGroups}</Mono>
						<CopyButton value={vm.setup.secret} iconOnly variant="ghost" size="xs" />
					</div>
				</div>
			</div>

			<form
				class={formClass}
				aria-labelledby={titleId}
				novalidate
				onsubmit={(event) => {
					event.preventDefault()
					void vm.submit()
				}}
			>
				<Field label={m.auth_totp_code()}>
					{#snippet children(a11y)}
						<CodeInput
							{...a11y}
							aria-invalid={vm.formError !== null}
							label={m.auth_totp_code()}
							value={vm.code}
							disabled={vm.isEnabling}
							oninput={vm.setCode}
							oncomplete={() => void vm.submit()}
						/>
					{/snippet}
				</Field>
				<Button
					type="submit"
					size="lg"
					fullWidth
					loading={vm.isEnabling}
					disabled={!vm.isCodeComplete}
				>
					{m.auth_2fa_submit()}
				</Button>
			</form>
		{:else if vm.isStarting}
			<LoadingState label={m.common_loading()} />
		{:else if vm.alreadyEnabled}
			<Banner title={m.auth_2fa_enabled_notice()} />
			<Button size="lg" fullWidth onclick={vm.start}>{m.auth_2fa_start()}</Button>
		{:else}
			<Button variant="secondary" size="lg" fullWidth onclick={vm.start}>
				{m.common_retry()}
			</Button>
		{/if}

		<div class={footer}>
			{#if vm.isRequired}
				<Button variant="ghost" size="sm" onclick={vm.logout}>{m.auth_logout()}</Button>
			{:else}
				<LinkButton href={resolve('/')} variant="ghost" size="sm">
					{vm.alreadyEnabled ? m.auth_back() : m.auth_2fa_skip()}
				</LinkButton>
			{/if}
		</div>
	</div>
</AuthLayout>
