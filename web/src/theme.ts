import type { GlobalThemeOverrides } from 'naive-ui'

// Design tokens — strictly derived from the GoDDI Web UI design file
// (dark iOS-blue theme, file 722862635550711).
export const designTokens = {
  blue: '#0a84ff',
  green: '#30d158',
  red: '#ff453a',
  orange: '#ff9f0a',
  purple: '#bf5af2',
  bgPage: '#1c1c1e',
  bgSurface: '#2c2c2e',
  bgSurfaceHover: '#333335',
  border: '#38383a',
  textPrimary: '#ffffff',
  textSecondary: '#d1d1d6',
  textMuted: '#a1a1a6',
  textTertiary: '#7c7c80',
  radiusCard: '10px',
  radiusControl: '6px',
  radiusPill: '4px',
} as const

export function appleTheme(dark: boolean): GlobalThemeOverrides {
  const t = designTokens
  // Both modes share the same design language; light mode is the
  // iOS-light mapping of the same tokens (white surfaces on #f5f5f7).
  const surface = dark ? t.bgSurface : '#ffffff'
  const pageBg = dark ? t.bgPage : '#f5f5f7'
  const separator = dark ? t.border : '#e5e5ea'
  return {
    common: {
      primaryColor: t.blue, primaryColorHover: '#3395ff', primaryColorPressed: '#0069d9', primaryColorSuppl: t.blue,
      infoColor: t.blue, successColor: t.green, errorColor: t.red, warningColor: t.orange,
      fontFamily: '"Noto Sans SC", -apple-system, BlinkMacSystemFont, "PingFang SC", "Helvetica Neue", sans-serif',
      fontSize: '14px', fontWeightStrong: '600', borderRadius: t.radiusControl, borderRadiusSmall: t.radiusPill,
      heightMedium: '36px', heightSmall: '30px',
      baseColor: surface, bodyColor: pageBg, cardColor: surface,
      modalColor: surface, popoverColor: surface, borderColor: separator,
      textColorBase: dark ? t.textPrimary : '#1d1d1f', textColor1: dark ? t.textPrimary : '#1d1d1f',
      textColor2: dark ? t.textSecondary : '#48484a', textColor3: dark ? t.textMuted : '#6e6e73',
    },
    Layout: {
      color: pageBg,
      headerColor: pageBg,
      siderColor: pageBg,
      headerBorderColor: separator,
      siderBorderColor: separator,
    },
    Card: {
      borderRadius: t.radiusCard,
      paddingMedium: '20px',
      titleFontSizeMedium: '16px',
      titleFontWeight: '600',
      color: surface,
      borderColor: separator,
    },
    Button: {
      fontWeight: '500',
      borderRadiusMedium: t.radiusControl,
      borderRadiusSmall: t.radiusControl,
    },
    Input: {
      color: dark ? t.bgSurface : '#fafafa',
      colorFocus: surface,
      border: `1px solid ${separator}`,
      borderHover: `1px solid ${dark ? '#48484a' : '#c7c7cc'}`,
      borderFocus: `1px solid ${t.blue}`,
      boxShadowFocus: 'none',
      borderRadius: t.radiusControl,
    },
    Menu: {
      itemHeight: '38px',
      borderRadius: t.radiusControl,
      itemColorActive: 'rgba(10, 132, 255, 0.12)',
      itemColorActiveHover: dark ? 'rgba(10, 132, 255, 0.2)' : 'rgba(10, 132, 255, 0.16)',
      itemColorActiveCollapsed: 'rgba(10, 132, 255, 0.12)',
      itemTextColorActive: dark ? t.textPrimary : t.blue,
      itemIconColorActive: dark ? t.textPrimary : t.blue,
      itemTextColorChildActive: dark ? t.textPrimary : t.blue,
      itemIconColorChildActive: dark ? t.textPrimary : t.blue,
      itemTextColor: dark ? t.textMuted : '#48484a',
      itemTextColorHover: dark ? t.textPrimary : '#1d1d1f',
      itemIconColorHover: dark ? t.textPrimary : '#1d1d1f',
    },
    DataTable: {
      thColor: surface,
      tdColor: surface,
      tdColorHover: dark ? t.bgSurfaceHover : '#f5f8fc',
      borderColor: separator,
      borderRadius: t.radiusCard,
      thTextColor: dark ? t.textTertiary : '#6e6e73',
      thFontWeight: '500',
      thPadding: '12px',
      tdPadding: '12px',
      fontSizeMedium: '13px',
      fontSizeSmall: '12px',
      emptyColor: dark ? t.textMuted : '#6e6e73',
    },
    Tag: { borderRadius: t.radiusPill },
    Tabs: { tabFontWeightActive: '600' },
    Switch: { railColorActive: t.blue },
    Pagination: { itemSize: '30px' },
  }
}
