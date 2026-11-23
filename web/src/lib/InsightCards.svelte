<script lang="ts">
	import type { Insights } from './api.ts';
	import { pct } from './dates.ts';

	let { insights }: { insights: Insights } = $props();

	const streak = $derived(insights.streak);

	// Average of the weeks that had something to resolve.
	const resolved = $derived(insights.trend.filter((a): a is number => a !== null));
	const avg = $derived(resolved.length ? resolved.reduce((a, b) => a + b, 0) / resolved.length : null);

	// Sparkline: one point per week, gaps where a week had nothing resolved.
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

	const sessions = $derived(
		insights.last_30.habits.filter((h) => h.plannable).reduce((n, h) => n + h.done, 0)
	);
	const days = $derived(insights.last_30.habits.filter((h) => !h.plannable));
</script>

<section class="cards">
	<div class="card">
		<div class="muted small">Streak</div>
		{#if streak}
			<div class="v">
				{streak.count}<span class="unit">{streak.count === 1 ? streak.unit.slice(0, -1) : streak.unit}</span>
			</div>
			<div class="sub">{streak.name}</div>
		{:else}
			<div class="v">0</div>
			<div class="sub">None running</div>
		{/if}
	</div>

	{#if avg !== null}
		<div class="card">
			<div class="muted small">8 weeks</div>
			<div class="v">{pct(avg)}<span class="unit">avg</span></div>
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
		</div>
	{/if}

	<div class="card">
		<div class="muted small">Last 30 days</div>
		<div class="v">{sessions}<span class="unit">{sessions === 1 ? 'session' : 'sessions'}</span></div>
		<div class="sub">
			{days.map((h) => `${h.name} ${h.done} ${h.done === 1 ? 'day' : 'days'}`).join(' · ') || ' '}
		</div>
	</div>
</section>
