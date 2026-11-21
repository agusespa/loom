<!-- Swipe sideways to change week. -->
<script lang="ts">
	import { onMount, tick, type Snippet } from 'svelte';
	import { motion } from '#lib/motion.ts';

	/**
	 * A sideways swipe anywhere on the page moves a week: right goes back, left goes forward, like
	 * turning pages. The wrapped content follows the finger, slides out and the new week slides in
	 * from the other side. Swiping towards a week you can't go to only gives a little.
	 */

	let {
		canPrev,
		canNext,
		onswipe,
		children
	}: {
		canPrev: boolean;
		canNext: boolean;
		/** -1 for the week before, 1 for the week after; resolves once the new week is in. */
		onswipe: (delta: -1 | 1) => Promise<unknown>;
		children: Snippet;
	} = $props();

	/** How far (px) the finger has to travel sideways before letting go changes week. */
	const THRESHOLD = 70;
	/** A quick flick changes week from this far (px)... */
	const FLICK = 30;
	/** ...when it's faster than this (px/ms). */
	const FLICK_SPEED = 0.4;
	/** Finger travel (px) ignored before deciding between a swipe and a scroll. */
	const SLOP = 8;
	/** Touches this close (px) to the screen's sides are left to the browser's own back gesture. */
	const EDGE = 24;

	let x = $state(0);
	let dragging = $state(false);
	let faded = $state(false);
	/** Moves without easing, to put the new week on the far side before it slides in. */
	let instant = $state(false);
	let busy = false;

	const wait = (ms: number) => new Promise((r) => setTimeout(r, ms));
	const frame = () => new Promise((r) => requestAnimationFrame(() => r(null)));

	async function commit(delta: -1 | 1) {
		busy = true;
		const dur = motion(160);
		const away = Math.min(innerWidth * 0.4, 160);
		// Forward pushes the page out to the left; back, to the right.
		x = -delta * away;
		faded = true;
		if (dur) await wait(dur);
		try {
			await onswipe(delta);
		} finally {
			instant = true;
			x = delta * away;
			await tick();
			await frame();
			instant = false;
			x = 0;
			faded = false;
			if (dur) await wait(dur);
			busy = false;
		}
	}

	onMount(() => {
		let startX: number | null = null;
		let startY = 0;
		let lastX = 0;
		let lastT = 0;
		let speed = 0;

		const reset = () => {
			startX = null;
			dragging = false;
			x = 0;
		};

		const start = (e: TouchEvent) => {
			const t = e.touches[0];
			// Leave an open day menu alone, and pinches.
			const skip =
				busy ||
				e.touches.length !== 1 ||
				t.clientX < EDGE ||
				t.clientX > innerWidth - EDGE ||
				document.querySelector('[role="menu"]');
			if (skip) return void (startX = null);
			startX = lastX = t.clientX;
			startY = t.clientY;
			lastT = e.timeStamp;
			speed = 0;
		};

		const move = (e: TouchEvent) => {
			if (startX === null) return;
			const t = e.touches[0];
			const dx = t.clientX - startX;
			const dy = t.clientY - startY;
			if (!dragging) {
				if (Math.max(Math.abs(dx), Math.abs(dy)) < SLOP) return;
				// Mostly up or down: a scroll (or a pull to refresh), not a swipe.
				if (Math.abs(dx) < Math.abs(dy) * 1.2) return void (startX = null);
				dragging = true;
			}
			// Keeps the page from scrolling up and down under a swipe.
			e.preventDefault();
			const dt = e.timeStamp - lastT;
			// Floored at a frame: events closer together than that would make any nudge a flick.
			speed = (t.clientX - lastX) / Math.max(dt, 16);
			lastX = t.clientX;
			lastT = e.timeStamp;
			const allowed = dx > 0 ? canPrev : canNext;
			x = allowed ? dx : Math.sign(dx) * Math.min(Math.abs(dx) * 0.25, 40);
		};

		const end = () => {
			if (startX === null) return;
			const dx = x;
			startX = null;
			if (!dragging) return;
			dragging = false;
			const delta = dx > 0 ? -1 : 1;
			const allowed = delta === -1 ? canPrev : canNext;
			const far = Math.abs(dx) >= THRESHOLD;
			const flick =
				Math.abs(dx) >= FLICK && Math.abs(speed) >= FLICK_SPEED && Math.sign(speed) === Math.sign(dx);
			if (allowed && (far || flick)) commit(delta);
			else x = 0;
		};

		addEventListener('touchstart', start, { passive: true });
		addEventListener('touchmove', move, { passive: false });
		addEventListener('touchend', end);
		addEventListener('touchcancel', reset);
		return () => {
			removeEventListener('touchstart', start);
			removeEventListener('touchmove', move);
			removeEventListener('touchend', end);
			removeEventListener('touchcancel', reset);
		};
	});
</script>

<div
	class="swipe"
	class:dragging
	class:instant
	class:faded
	class:moved={x !== 0}
	style:--x="{x}px"
	data-testid="swipe-nav"
>
	{@render children()}
</div>

<style>
	.swipe {
		transition:
			transform 0.16s ease-out,
			opacity 0.16s ease-out;
	}

	/* Only transformed while moving: at rest it mustn't become the open day menu's containing block. */
	.swipe.moved {
		transform: translateX(var(--x));
	}

	.swipe.faded {
		opacity: 0;
	}

	/* Follow the finger exactly; ease only on release. */
	.swipe.dragging,
	.swipe.instant {
		transition: none;
	}

	@media (prefers-reduced-motion: reduce) {
		.swipe {
			transition: none;
		}
	}
</style>
