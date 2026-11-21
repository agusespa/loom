<script lang="ts">
	import { onMount } from 'svelte';
	import { fetchWeek, type Week } from '#lib/api.ts';
	import { addDays, ago, fmt, isoWeek, pct } from '#lib/dates.ts';
	import GoalList from '#lib/GoalList.svelte';
	import HabitGrid from '#lib/HabitGrid.svelte';

	// Everything comes from GET /api/week, which n8n keeps up to date.

	let week = $state<Week | null>(null);
	let current = $state<string | null>(null); // the Monday shown, null = this week
	let latestStart = $state<string | null>(null); // this week's Monday, from the server
	let banner = $state<'offline' | 'expired' | { status: number } | null>(null);
	let loading = $state(true);

	const isCurrent = $derived(!!week && (current === null || week.start === latestStart));

	async function load() {
		const res = await fetchWeek(current);
		loading = false;
		if (res.kind === 'ok') {
			if (current === null) latestStart = res.week.start;
			week = res.week;
			banner = null;
		} else if (res.kind === 'error') {
			banner = { status: res.status };
		} else {
			banner = res.kind;
		}
	}

	function go(delta: number) {
		const base = current ?? latestStart;
		if (!base) return;
		const next = addDays(base, delta * 7);
		current = latestStart && next >= latestStart ? null : next;
		load();
	}

	function headline(w: Week): string {
		const { done, missed, planned, adherence } = w.summary;
		const now = isCurrent;
		if (done + missed + planned === 0) return now ? 'Nothing planned yet' : 'Quiet week';
		if (adherence === null) return planned ? `${planned} planned` : 'Week ahead';
		if (adherence >= 0.85) return now ? 'Great week so far' : 'Great week';
		if (adherence >= 0.6) return now ? 'Good week so far' : 'Good week';
		return now ? 'Room to catch up' : 'Tough week';
	}

	onMount(() => {
		load();
		const onVisible = () => document.visibilityState === 'visible' && load();
		document.addEventListener('visibilitychange', onVisible);
		return () => document.removeEventListener('visibilitychange', onVisible);
	});
</script>

<main>
	<header class="top">
		<button class="nav" id="prev" aria-label="Previous week" disabled={!week} onclick={() => go(-1)}>‹</button
		>
		<div class="top-mid">
			<div class="muted small">
				{#if week}
					Week {isoWeek(week.start)} · {fmt(week.start, { day: 'numeric', month: 'short' })}–{fmt(week.end, {
						day: 'numeric',
						month: 'short'
					})}
				{:else}&nbsp;{/if}
			</div>
			<h1>{week ? headline(week) : loading ? 'Loading…' : 'Loom'}</h1>
		</div>
		<div class="score">
			{#if week}
				{@const a = week.summary.adherence}
				<div class="big" class:good={a !== null && a >= 0.7} class:bad={a !== null && a < 0.5}>{pct(a)}</div>
				<div class="muted small">
					{week.prev_adherence === null ? ' ' : `vs ${pct(week.prev_adherence)} last wk`}
				</div>
			{:else}
				<div class="big">–</div>
				<div class="muted small">&nbsp;</div>
			{/if}
		</div>
		<button class="nav" id="next" aria-label="Next week" disabled={!week || isCurrent} onclick={() => go(1)}
			>›</button
		>
	</header>

	{#if banner}
		<div class="banner" role="status">
			{#if banner === 'offline'}
				Couldn't reach Loom. Check your connection.
			{:else if banner === 'expired'}
				Your session expired. <a href="/" onclick={() => location.reload()}>Sign in again</a>
			{:else}
				Couldn't load this week (HTTP {banner.status}).
			{/if}
		</div>
	{/if}

	{#if week}
		<section class="cards">
			{#each week.habits as h (h.key)}
				<div class="card">
					<div class="muted small">{h.name}</div>
					<div class="v">
						{h.done}<span
							>{h.plannable
								? `/${h.days.filter((d) => d.status).length}`
								: h.done === 1
									? ' day'
									: ' days'}</span
						>
					</div>
				</div>
			{/each}
		</section>

		<section>
			<div class="sec"><span>Habits</span><span class="muted">Tap a day for details</span></div>
			<HabitGrid habits={week.habits} today={week.today} {isCurrent} />
		</section>

		<section>
			<div class="sec"><span>Short-term goals</span><span class="muted">From Notion</span></div>
			<GoalList goals={week.goals.short_term} today={week.today} />
		</section>

		<section>
			<div class="sec"><span>Perennial goals</span><span class="muted">From Notion</span></div>
			<GoalList goals={week.goals.perennial} today={week.today} />
		</section>

		<footer class="muted small">
			{week.updated_at ? `Updated ${ago(week.updated_at)}` : 'No data from n8n yet'} · v{__APP_VERSION__}
		</footer>
	{/if}
</main>
