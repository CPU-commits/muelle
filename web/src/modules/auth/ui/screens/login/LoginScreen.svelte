<script lang="ts">
	import { page } from '$app/state'
	import { m } from '@/shared/ui/i18n/paraglide/messages'
	import { AuthLayout, Banner, Button, CodeInput, Field, firstError, Input } from '@/shared/ui'
	import { AuthHeading } from '../../components/authHeading'
	import {
		type LoginStep,
		LoginViewModel,
		validateRequiredField,
	} from './Login.viewModel.svelte.ts'
	import { codeFooter, fields, form as formClass, note } from './styles.css'

	let { initialStep = 'password' }: { initialStep?: LoginStep } = $props()

	const vm = new LoginViewModel(() => initialStep)
	const titleId = $props.id()
</script>

<AuthLayout appName="muelle" host={page.url.host}>
	{#if vm.step === 'password'}
		<form
			class={formClass}
			aria-labelledby={titleId}
			novalidate
			onsubmit={(event) => {
				event.preventDefault()
				event.stopPropagation()
				void vm.form.handleSubmit()
			}}
		>
			<AuthHeading
				id={titleId}
				title={m.auth_login_title()}
				subtitle={m.auth_login_subtitle()}
			/>

			{#if vm.formError}
				<Banner tone="error" title={vm.formError} />
			{/if}

			<div class={fields}>
				<vm.form.Field
					name="username"
					validators={{
						onBlur: ({ value }) => validateRequiredField(value),
						onSubmit: ({ value }) => validateRequiredField(value),
					}}
				>
					{#snippet children(field)}
						<Field
							label={m.auth_username()}
							error={firstError(field.state.meta.errors)}
						>
							{#snippet children(a11y)}
								<Input
									{...a11y}
									inputSize="lg"
									name={field.name}
									autocomplete="username"
									autocapitalize="none"
									spellcheck={false}
									value={field.state.value}
									disabled={vm.isPending}
									onblur={field.handleBlur}
									oninput={(event) =>
										field.handleChange(event.currentTarget.value)}
								/>
							{/snippet}
						</Field>
					{/snippet}
				</vm.form.Field>

				<vm.form.Field
					name="password"
					validators={{
						onBlur: ({ value }) => validateRequiredField(value),
						onSubmit: ({ value }) => validateRequiredField(value),
					}}
				>
					{#snippet children(field)}
						<Field
							label={m.auth_password()}
							error={firstError(field.state.meta.errors)}
						>
							{#snippet children(a11y)}
								<Input
									{...a11y}
									inputSize="lg"
									type="password"
									name={field.name}
									autocomplete="current-password"
									value={field.state.value}
									disabled={vm.isPending}
									onblur={field.handleBlur}
									oninput={(event) =>
										field.handleChange(event.currentTarget.value)}
								/>
							{/snippet}
						</Field>
					{/snippet}
				</vm.form.Field>
			</div>

			<Button type="submit" size="lg" fullWidth loading={vm.isPending}>
				{m.auth_login_submit()}
			</Button>
		</form>
	{:else}
		<form
			class={formClass}
			aria-labelledby={titleId}
			novalidate
			onsubmit={(event) => {
				event.preventDefault()
				void vm.submitCode()
			}}
		>
			<AuthHeading
				id={titleId}
				title={m.auth_totp_title()}
				subtitle={m.auth_totp_subtitle()}
			/>

			{#if vm.formError}
				<Banner tone="error" title={vm.formError} />
			{/if}

			<Field label={m.auth_totp_code()}>
				{#snippet children(a11y)}
					<CodeInput
						{...a11y}
						aria-invalid={vm.formError !== null}
						label={m.auth_totp_code()}
						value={vm.code}
						disabled={vm.isPending}
						focusOnMount
						oninput={vm.setCode}
						oncomplete={() => void vm.submitCode()}
					/>
				{/snippet}
			</Field>

			<Button
				type="submit"
				size="lg"
				fullWidth
				loading={vm.isPending}
				disabled={!vm.isCodeComplete}
			>
				{m.auth_totp_submit()}
			</Button>

			<div class={codeFooter}>
				<Button variant="ghost" size="sm" onclick={vm.backToPassword}
					>{m.auth_back()}</Button
				>
				<p class={note}>{m.auth_totp_lost_phone()}</p>
			</div>
		</form>
	{/if}
</AuthLayout>
