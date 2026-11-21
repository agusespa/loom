<script lang="ts">
	import '../app.css';
	import { onMount } from 'svelte';
	import { fly } from 'svelte/transition';
	import { installErrorToasts } from '#lib/errors.ts';
	import { motion } from '#lib/motion.ts';
	import { runToastAction, toast } from '#lib/toast.svelte.ts';

	let { children } = $props();

	onMount(installErrorToasts);
</script>

{@render children()}

<!-- One short message at a time, as in Gympass: uncaught errors land here instead of failing silently. -->
{#if toast.current}
	{@const t = toast.current}
	<div
		class="toast"
		role="status"
		in:fly={{ y: 24, duration: motion(220) }}
		out:fly={{ y: 12, duration: motion(160) }}
	>
		<span>{t.message}</span>
		{#if t.action}
			<button onclick={runToastAction}>{t.action.label}</button>
		{/if}
	</div>
{/if}

<style>
	.toast {
		position: fixed;
		left: 50%;
		bottom: calc(env(safe-area-inset-bottom) + 16px);
		transform: translateX(-50%);
		z-index: 20;
		display: flex;
		align-items: center;
		gap: 0.75rem;
		width: min(calc(100% - 32px), 400px);
		min-height: 44px;
		padding: 0.4rem 0.4rem 0.4rem 1rem;
		border-radius: 14px;
		background: var(--glass);
		-webkit-backdrop-filter: var(--glass-filter);
		backdrop-filter: var(--glass-filter);
		box-shadow: var(--glass-edge), var(--glass-shadow);
		font-size: 0.95rem;
	}

	.toast span {
		flex: 1;
	}

	.toast button {
		border: 0;
		background: transparent;
		color: var(--accent);
		font: inherit;
		font-weight: 600;
		padding: 0.5rem 0.75rem;
		cursor: pointer;
	}
</style>
