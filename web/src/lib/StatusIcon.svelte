<script lang="ts">
	import type { Status } from './api.ts';

	// One stroke set so done, missed and planned read at the same size and weight.
	let { status, size = 14 }: { status: Status; size?: number } = $props();
</script>

<svg
	class="status-icon"
	viewBox="0 0 16 16"
	width={size}
	height={size}
	fill="none"
	stroke="currentColor"
	stroke-width="2"
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden="true"
>
	{#if status === 'done'}
		<path d="M3.5 8.5l3 3 6-7" />
	{:else if status === 'missed'}
		<path d="M4.5 4.5l7 7M11.5 4.5l-7 7" />
	{:else}
		<circle cx="8" cy="8" r="5.75" stroke-width="1.6" />
		<path d="M8 5v3.25l2 1.25" stroke-width="1.6" />
	{/if}
</svg>
