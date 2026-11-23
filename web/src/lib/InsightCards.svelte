<script lang="ts">
	import type { Insights } from './api.ts';
	import { slide } from 'svelte/transition';
	import { delta, pct } from './dates.ts';
	import { motion } from './motion.ts';

	let { insights }: { insights: Insights } = $props();

	// Tapping the cards shows every habit's streak and adherence. Closed, each card has the edges of
	// more cards peeking out under it, as if stacked: the hint that there's more.
	let open = $state(false);

	const streak = $derived(insights.streak);
	const change = $derived(delta(insights.adherence, insights.prev_adherence));

	// Sparkline: one point per week, gaps where a week had nothing to count.
	const W = 72;
	const H = 18;
	const points = $derived(
		insights.trend.map((a, i) =>
			a === null ? null : { x: (i / (insights.trend.length - 1)) * W, y: 2 + (1 - a) * (H - 4) }
		)
	);
	type Point = { x: number; y: number };
	const segments = $derived.by(() => {
		const out: Point[][] = [[]];
		for (const p of points) {
			if (p) out.at(-1)!.push(p);
			else if (out.at(-1)!.length) out.push([]);
		}
		return out.filter((s) => s.length);
	});
	const last = $derived(points.at(-1));

	function unit(n: number, u: 'sessions' | 'days'): string {
		return n === 1 ? u.slice(0, -1) : u;
	}
</script>

{#snippet trend(d: number | null, prev: number | null)}
	{#if d !== null}
		<span
			class="trend"
			class:up={d > 0}
			class:down={d < 0}
			title="vs {pct(prev)} the 8 weeks before"
			aria-label="{d > 0 ? 'up' : d < 0 ? 'down' : 'level'} {Math.abs(d)} points on the 8 weeks before"
			><span aria-hidden="true">{d > 0 ? '▲' : d < 0 ? '▼' : '='}</span>{d === 0 ? '' : Math.abs(d)}</span
		>
	{/if}
{/snippet}

<button
	type="button"
	class="cards"
	class:open
	aria-expanded={open}
	aria-controls="habit-insights"
	aria-label="{open ? 'Hide' : 'Show'} streaks and adherence by habit"
	onclick={() => (open = !open)}
>
	<span class="card">
		<span class="muted small">Longest streak</span>
		{#if streak}
			<span class="v">{streak.count}<span class="unit">{unit(streak.count, streak.unit)}</span></span>
			<span class="sub">{streak.name}</span>
		{:else}
			<span class="v">0</span>
			<span class="sub">None running</span>
		{/if}
	</span>

	{#if insights.adherence !== null}
		<span class="card">
			<span class="muted small">8 weeks</span>
			<span class="v">{pct(insights.adherence)}{@render trend(change, insights.prev_adherence)}</span>
			<svg
				class="spark"
				viewBox="0 0 {W} {H}"
				width={W}
				height={H}
				role="img"
				aria-label="Adherence over the last 8 weeks"
			>
				<line x1="0" x2={W} y1={H / 2} y2={H / 2} class="mid" />
				{#each segments as seg, i (i)}
					{#if seg.length > 1}
						<polyline points={seg.map((p) => `${p.x},${p.y}`).join(' ')} />
					{:else}
						<circle cx={seg[0].x} cy={seg[0].y} r="1.5" />
					{/if}
				{/each}
				{#if last}<circle class="now" cx={last.x} cy={last.y} r="2.5" />{/if}
			</svg>
		</span>
	{/if}
</button>

{#if open}
	<table id="habit-insights" class="habit-insights" transition:slide={{ duration: motion(220) }}>
		<thead>
			<tr>
				<th scope="col">Habit</th>
				<th scope="col">Streak</th>
				<th scope="col">8 weeks</th>
			</tr>
		</thead>
		<tbody>
			{#each insights.habits as h (h.key)}
				<tr>
					<th scope="row">{h.name}</th>
					<td>{h.streak ? `${h.streak} ${unit(h.streak, h.unit)}` : '0'}</td>
					<td>{pct(h.adherence)}{@render trend(delta(h.adherence, h.prev_adherence), h.prev_adherence)}</td>
				</tr>
			{/each}
		</tbody>
	</table>
{/if}
