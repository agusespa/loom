/** The phone asked for less motion: animations are skipped (made instant). */
export const reduceMotion =
	typeof matchMedia === 'function' && matchMedia('(prefers-reduced-motion: reduce)').matches;

/** An animation's length in ms, or 0 when the phone asks for less motion. */
export const motion = (ms: number) => (reduceMotion ? 0 : ms);
