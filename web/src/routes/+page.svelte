<script lang="ts">
	import { version } from '$app/env';
	import { onMount } from 'svelte';
	import { fetchWeek, putDay, type Status, type Week } from '#lib/api.ts';
	import { addDays, ago, fmt, isoWeek, pct } from '#lib/dates.ts';
	import GoalList from '#lib/GoalList.svelte';
	import HabitGrid from '#lib/HabitGrid.svelte';
	import InsightCards from '#lib/InsightCards.svelte';

	// Everything comes from GET /api/week. n8n keeps it up to date; days can
	// also be corrected here.

	let week = $state<Week | null>(null);
	let current = $state<string | null>(null); // the Monday shown, null = this week
	let latestStart = $state<string | null>(null); // this week's Monday, from the server
	let banner = $state<'offline' | 'expired' | { status: number } | null>(null);
	let loading = $state(true);

	// SvelteKit's build version is the build's timestamp; it tells two builds of one release apart.
	const builtMs = Number(version);
	const builtAt = Number.isFinite(builtMs)
		? new Date(builtMs).toLocaleString(undefined, {
				day: 'numeric',
				month: 'short',
				year: 'numeric',
				hour: 'numeric',
				minute: '2-digit'
			})
		: null;

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

	async function edit(habit: string, date: string, status: Status | null): Promise<string | null> {
		const res = await putDay(habit, date, status);
		if (res.kind === 'ok') {
			await load();
			return null;
		}
		if (res.kind === 'expired') {
			banner = 'expired';
			return 'Sign in again to save.';
		}
		return res.kind === 'offline' ? "Couldn't reach Loom." : res.message;
	}

	function go(delta: number) {
		const base = current ?? latestStart;
		if (!base) return;
		const next = addDays(base, delta * 7);
		current = latestStart && next >= latestStart ? null : next;
		load();
	}

	/** Good from 60%, where the headline starts saying "Good week"; bad under 40%. */
	function grade(a: number | null): string {
		if (a === null) return '';
		return a >= 0.6 ? 'good' : a >= 0.4 ? 'ok' : 'bad';
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
	<div class="band">
		<header class="top">
			<button class="nav" id="prev" aria-label="Previous week" disabled={!week} onclick={() => go(-1)}
				>‹</button
			>
			<div class="top-mid">
				<div class="muted small">
					{#if week}
						Week {isoWeek(week.start)} · {fmt(week.start, { day: 'numeric', month: 'short' })}–{fmt(
							week.end,
							{
								day: 'numeric',
								month: 'short'
							}
						)}
					{:else}&nbsp;{/if}
				</div>
				<h1>{week ? headline(week) : loading ? 'Loading…' : 'Loom'}</h1>
			</div>
			<div class="score">
				{#if week}
					{@const a = week.summary.adherence}
					<div class="big {grade(a)}">{pct(a)}</div>
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
		{#if week}
			<InsightCards insights={week.insights} />
		{/if}
	</div>

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
		<section>
			<div class="sec"><span>Habits</span><span class="muted">Tap a day to edit</span></div>
			<HabitGrid habits={week.habits} today={week.today} {isCurrent} onedit={edit} />
		</section>

		<section>
			<div class="sec"><span>Short-term goals</span><span class="muted">From Notion</span></div>
			<GoalList goals={week.goals.short_term} today={week.today} />
		</section>

		<section>
			<div class="sec"><span>Perennial goals</span><span class="muted">From Notion</span></div>
			<GoalList goals={week.goals.perennial} today={week.today} />
		</section>
	{/if}

	<!-- Always shown, so you can tell which build is installed even when loading fails. -->
	<footer class="muted small">
		{#if week}<p>{week.updated_at ? `Updated ${ago(week.updated_at)}` : 'Nothing logged yet'}</p>{/if}
		<p class="version" data-testid="app-version">
			Version {__APP_VERSION__}{#if builtAt}<br />Built {builtAt}{/if}
		</p>
	</footer>
</main>
