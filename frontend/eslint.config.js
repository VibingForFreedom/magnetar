import js from '@eslint/js';
import ts from 'typescript-eslint';
import svelte from 'eslint-plugin-svelte';
import svelteParser from 'svelte-eslint-parser';
import globals from 'globals';

export default [
	js.configs.recommended,
	...ts.configs.recommended,
	...svelte.configs['flat/recommended'],
	{
		languageOptions: {
			globals: {
				...globals.browser,
				...globals.node
			}
		}
	},
	{
		files: ['**/*.svelte', '**/*.svelte.ts'],
		languageOptions: {
			parser: svelteParser,
			parserOptions: {
				parser: ts.parser
			}
		}
	},
	{
		rules: {
			// Relax rules that conflict with Svelte 5 / SvelteKit patterns
			'@typescript-eslint/no-unused-vars': ['warn', {
				argsIgnorePattern: '^_',
				varsIgnorePattern: '^\\$\\$'
			}],
			'@typescript-eslint/no-explicit-any': 'warn',
			'no-undef': 'off',  // TypeScript handles this
			'no-console': 'warn',
			eqeqeq: ['error', 'always'],
			'no-var': 'error',
			'no-debugger': 'error'
		}
	},
	{
		// Svelte 5 $props() uses `let` for destructuring — prefer-const is wrong here
		files: ['**/*.svelte'],
		rules: {
			'prefer-const': 'off'
		}
	},
	{
		ignores: [
			'build/**',
			'.svelte-kit/**',
			'node_modules/**'
		]
	}
];
