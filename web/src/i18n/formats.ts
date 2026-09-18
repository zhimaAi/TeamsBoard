export const datetimeFormats = {
  'zh-CN': {
    short: {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    },
    long: {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    },
  },
  'en-US': {
    short: {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
    },
    long: {
      year: 'numeric',
      month: 'long',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
    },
  },
} as const

export const numberFormats = {
  'zh-CN': {
    decimal: {
      style: 'decimal',
      maximumFractionDigits: 2,
    },
    percent: {
      style: 'percent',
      useGrouping: false,
    },
  },
  'en-US': {
    decimal: {
      style: 'decimal',
      maximumFractionDigits: 2,
    },
    percent: {
      style: 'percent',
      useGrouping: false,
    },
  },
} as const

export type DateTimeSchema = (typeof datetimeFormats)['zh-CN']
export type NumberSchema = (typeof numberFormats)['zh-CN']
