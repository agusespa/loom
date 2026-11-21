import { showToast } from './toast.svelte.ts';

/**
 * Tell the user something failed instead of leaving a tap that silently does nothing.
 * The error's own message is included: this is a personal app, and on a phone the toast
 * is the only place to see why.
 */
export function reportError(e: unknown) {
	console.error(e);
	const detail = e instanceof Error ? e.message : typeof e === 'string' ? e : '';
	showToast({ message: detail ? `Something went wrong: ${detail}` : 'Something went wrong.' });
}

let installed = false;

/**
 * Catch everything nobody else handled: a thrown click handler or a rejected async one.
 * Loading and saving handle their own failures (the banner and the day menu), so they never land here.
 */
export function installErrorToasts() {
	if (installed) return;
	installed = true;
	window.addEventListener('error', (ev) => reportError(ev.error ?? ev.message));
	window.addEventListener('unhandledrejection', (ev) => reportError(ev.reason));
}
