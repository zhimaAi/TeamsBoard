import type { ConfigProviderProps } from 'ant-design-vue/es/config-provider'

/** Client version number */
export const CLIENT_VERSION = '0.1.0'

export const THEME_COLORS = {
  primary: '#3157E2',
  primarySoft: '#E5EFFF',
  textStrong: '#1D1D1F',
  textPrimary: '#262626',
  textBody: '#595959',
  textSecondary: '#8C8C8C',
  textDisabled: '#BFBFBF',
  canvas: '#F5F6F8',
  surface: '#FFFFFF',
  mutedSurface: '#EDEFF2',
  subtleSurface: '#FBFBFC',
  border: '#D9D9D9',
  divider: '#F0F0F0',
  error: '#FB363F',
  errorSurface: '#FEF2F2',
  warning: '#ED744A',
  warningSurface: '#FFF5E5',
  success: '#16A34A',
  successSurface: '#F0FDF4',
} as const

export const PRIMARY_COLOR = THEME_COLORS.primary

export const BORDER_RADIUS = 6

const FONT_FAMILY =
  "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, 'Fira Sans', 'Droid Sans', 'Helvetica Neue', sans-serif"

const SURFACE_SHADOW =
  '0 1px 2px rgba(16, 24, 40, 0.03), 0 2px 4px rgba(34, 52, 79, 0.03)'
const FLOATING_SHADOW =
  '0 6px 30px 5px rgba(0, 0, 0, 0.05), 0 16px 24px 2px rgba(0, 0, 0, 0.04), 0 8px 10px -5px rgba(0, 0, 0, 0.08)'
const MODAL_SHADOW = '0 4px 16px rgba(0, 0, 0, 0.16)'

const MANAGEMENT_CONTROL_TOKEN = {
  controlHeight: 40,
  borderRadius: 12,
  borderRadiusLG: 12,
  fontSize: 16,
  lineHeight: 1.5,
} as const

type AntThemeConfig = NonNullable<ConfigProviderProps['theme']>

export const themeConfig = {
  token: {
    colorPrimary: PRIMARY_COLOR,
    colorInfo: PRIMARY_COLOR,
    colorSuccess: THEME_COLORS.success,
    colorWarning: THEME_COLORS.warning,
    colorError: THEME_COLORS.error,
    colorBgBase: THEME_COLORS.surface,
    colorBgLayout: THEME_COLORS.canvas,
    colorBgContainer: THEME_COLORS.surface,
    colorBgElevated: THEME_COLORS.surface,
    colorTextBase: THEME_COLORS.textPrimary,
    colorText: THEME_COLORS.textPrimary,
    colorTextHeading: THEME_COLORS.textStrong,
    colorTextLabel: THEME_COLORS.textPrimary,
    colorTextSecondary: THEME_COLORS.textBody,
    colorTextTertiary: THEME_COLORS.textSecondary,
    colorTextQuaternary: THEME_COLORS.textDisabled,
    colorTextDescription: THEME_COLORS.textSecondary,
    colorTextPlaceholder: THEME_COLORS.textSecondary,
    colorTextDisabled: THEME_COLORS.textDisabled,
    colorIcon: THEME_COLORS.textSecondary,
    colorIconHover: THEME_COLORS.textBody,
    colorLink: PRIMARY_COLOR,
    colorHighlight: THEME_COLORS.error,
    colorBorder: THEME_COLORS.border,
    colorBorderSecondary: THEME_COLORS.divider,
    colorSplit: THEME_COLORS.divider,
    colorPrimaryBg: THEME_COLORS.primarySoft,
    colorErrorBg: THEME_COLORS.errorSurface,
    colorWarningBg: THEME_COLORS.warningSurface,
    colorSuccessBg: THEME_COLORS.successSurface,
    colorFillTertiary: THEME_COLORS.mutedSurface,
    colorFillQuaternary: THEME_COLORS.subtleSurface,
    colorBgTextHover: '#F0F1F3',
    colorBgTextActive: '#E9E9EB',
    controlItemBgActive: THEME_COLORS.primarySoft,
    controlOutline: 'rgba(49, 87, 226, 0.15)',
    controlOutlineWidth: 2,
    borderRadius: BORDER_RADIUS,
    borderRadiusXS: 4,
    borderRadiusSM: 6,
    borderRadiusLG: 12,
    controlHeight: 32,
    controlHeightLG: 40,
    fontFamily: FONT_FAMILY,
    fontSize: 14,
    fontSizeSM: 12,
    fontSizeLG: 16,
    fontSizeHeading3: 24,
    fontSizeHeading4: 20,
    fontSizeHeading5: 16,
    fontWeightStrong: 600,
    lineHeight: 22 / 14,
    lineHeightSM: 20 / 12,
    lineHeightLG: 1.5,
    lineHeightHeading3: 32 / 24,
    lineHeightHeading4: 28 / 20,
    lineHeightHeading5: 1.5,
    motionDurationFast: '0.18s',
    motionDurationMid: '0.2s',
    boxShadow: SURFACE_SHADOW,
    boxShadowSecondary: FLOATING_SHADOW,
    boxShadowTertiary: SURFACE_SHADOW,
  },
  components: {
    Button: {
      controlHeight: 32,
      borderRadius: BORDER_RADIUS,
      controlPaddingHorizontal: 16,
      fontSize: 14,
    },
    Input: MANAGEMENT_CONTROL_TOKEN,
    InputNumber: MANAGEMENT_CONTROL_TOKEN,
    Select: MANAGEMENT_CONTROL_TOKEN,
    Cascader: MANAGEMENT_CONTROL_TOKEN,
    DatePicker: MANAGEMENT_CONTROL_TOKEN,
    TreeSelect: MANAGEMENT_CONTROL_TOKEN,
    Mentions: MANAGEMENT_CONTROL_TOKEN,
    Card: {
      borderRadiusLG: 12,
      paddingLG: 24,
      colorBorderSecondary: THEME_COLORS.border,
    },
    Modal: {
      borderRadiusLG: 12,
      boxShadowSecondary: MODAL_SHADOW,
      paddingLG: 24,
    },
    Tag: {
      borderRadiusSM: 999,
      fontSizeSM: 12,
    },
    Segmented: {
      borderRadius: 12,
      borderRadiusSM: 10,
      colorFillSecondary: THEME_COLORS.mutedSurface,
      colorBgLayout: THEME_COLORS.surface,
      controlItemBgActive: THEME_COLORS.surface,
      boxShadow: SURFACE_SHADOW,
    },
  },
} satisfies AntThemeConfig
