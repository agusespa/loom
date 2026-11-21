/// <reference types="vitest/config" />
import adapter from '@sveltejs/adapter-static';
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig, loadEnv } from 'vite';

export default defineConfig(({ mode }) => ({
	define: {
		// The release shown in the footer: the image build passes the release tag as APP_VERSION.
		__APP_VERSION__: JSON.stringify(loadEnv(mode, '.', 'APP_').APP_VERSION?.replace(/^v/, '') || 'dev')
	},
	plugins: [
		sveltekit({
			compilerOptions: {
				// Force runes mode for the project, except for libraries. Can be removed in svelte 6.
				runes: ({ filename }) => (filename.split(/[/\\]/).includes('node_modules') ? undefined : true)
			},
			// Single-page app: the Go server serves index.html for every non-file path.
			adapter: adapter({ fallback: 'index.html' })
		})
	],
	server: { proxy: { '/api': 'http://127.0.0.1:8080' } },
	test: {
		include: ['src/**/*.test.ts']
	}
}));
