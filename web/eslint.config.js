import js from '@eslint/js'
import { defineConfig } from 'eslint/config'
import eslintConfigPrettier from 'eslint-config-prettier/flat'
import pluginVue from 'eslint-plugin-vue'
import globals from 'globals'
import tseslint from 'typescript-eslint'

export default defineConfig([
  {
    ignores: ['dist/**', 'node_modules/**', 'components.d.ts'],
  },
  {
    files: ['src/**/*.{ts,tsx,vue}', 'vite.config.ts'],
    extends: [
      js.configs.recommended,
      ...tseslint.configs.recommended,
      ...pluginVue.configs['flat/essential'],
    ],
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
      parserOptions: {
        parser: tseslint.parser,
      },
    },
  },
  {
    files: ['src/**/*.{ts,tsx,vue}'],
    languageOptions: {
      globals: globals.browser,
    },
  },
  {
    files: ['vite.config.ts'],
    languageOptions: {
      globals: globals.node,
    },
  },
  eslintConfigPrettier,
])
