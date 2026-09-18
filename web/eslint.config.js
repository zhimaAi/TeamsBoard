import js from '@eslint/js'
import vueI18n from '@intlify/eslint-plugin-vue-i18n'
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
  ...vueI18n.configs['flat/recommended'],
  {
    settings: {
      'vue-i18n': {
        localeDir: {
          pattern: './src/locales/*/*.json',
          localeKey: 'path',
          localePattern: '[/\\\\](?<locale>[A-Za-z]{2}-[A-Za-z]{2})[/\\\\]',
        },
        messageSyntaxVersion: '^11.0.0',
      },
    },
  },
  {
    files: ['src/**/*.{vue,json}'],
    rules: {
      // 语言资源按文件名在运行时组装 namespace，插件无法从文件名推导 key 前缀。
      '@intlify/vue-i18n/no-missing-keys': 'off',
      '@intlify/vue-i18n/key-format-style': ['error', 'camelCase'],
      '@intlify/vue-i18n/no-duplicate-keys-in-locale': 'error',
      '@intlify/vue-i18n/no-html-messages': 'error',
      '@intlify/vue-i18n/no-missing-keys-in-other-locales': 'error',
      '@intlify/vue-i18n/no-raw-text': [
        'error',
        {
          ignoreText: [
            'TeamsBoard',
            'Agent',
            'CLI',
            'API',
            'Git',
            'SSH',
            'Docker Compose',
            'MySQL',
            'PostgreSQL',
            'PgSQL',
            'OpenAI',
            'Anthropic',
            'Ollama',
            'Markdown',
            'Params',
            'Header',
            'Headers',
            'Body',
            'JSON',
            'Text',
            'x-www-form-urlencoded',
            'multipart/form-data',
            'Temperature',
            'API Key',
            'Bearer Token',
            'Basic Auth',
            'Cookie',
            'Query',
            'GT',
            'COMMAND PALETTE',
            'none',
            'key',
            'field',
            'value',
            'Content-Type',
            'application/json',
          ],
        },
      ],
      '@intlify/vue-i18n/valid-message-syntax': 'error',
    },
  },
  {
    // These components have no runtime import and are retained only as migration references.
    files: ['src/components/CreateTaskModal.vue', 'src/components/TaskExecutionConsole.vue'],
    rules: {
      '@intlify/vue-i18n/no-raw-text': 'warn',
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
