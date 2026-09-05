import type { GlobalThemeOverrides } from 'naive-ui'

export function appleTheme(dark: boolean): GlobalThemeOverrides {
  const surface = dark ? '#242426' : '#ffffff'
  const separator = dark ? '#3a3a3c' : '#e5e5ea'
  return {
    common: {
      primaryColor: '#007aff', primaryColorHover: '#3395ff', primaryColorPressed: '#0062cc', primaryColorSuppl: '#007aff',
      fontFamily: '-apple-system, BlinkMacSystemFont, "Helvetica Neue", "PingFang SC", sans-serif',
      fontSize: '14px', fontWeightStrong: '600', borderRadius: '8px', borderRadiusSmall: '6px',
      baseColor: surface, bodyColor: dark ? '#1c1c1e' : '#f5f5f7', cardColor: surface,
      modalColor: surface, popoverColor: surface, borderColor: separator,
      textColorBase: dark ? '#f5f5f7' : '#1d1d1f', textColor1: dark ? '#f5f5f7' : '#1d1d1f',
      textColor2: dark ? '#d1d1d6' : '#48484a', textColor3: dark ? '#a1a1a6' : '#6e6e73',
      heightMedium: '36px', heightSmall: '30px',
    },
    Layout: { color: dark ? '#1c1c1e' : '#f5f5f7', headerColor: surface, siderColor: dark ? '#202022' : '#eff0f3' },
    Card: { borderRadius: '16px', paddingMedium: '24px', titleFontSizeMedium: '16px', titleFontWeight: '600' },
    Button: { fontWeight: '500', borderRadiusMedium: '8px', borderRadiusSmall: '7px' },
    Input: { color: dark ? '#2c2c2e' : '#fafafa', colorFocus: surface, border: `1px solid ${separator}` },
    Menu: {
      itemHeight: '44px', borderRadius: '9px', itemColorActive: dark ? '#14385e' : '#dceaff',
      itemColorActiveHover: dark ? '#194675' : '#d3e4ff', itemTextColorActive: dark ? '#69adff' : '#0067da',
      itemIconColorActive: dark ? '#69adff' : '#007aff', itemTextColorChildActive: dark ? '#69adff' : '#0067da',
      itemIconColorChildActive: dark ? '#69adff' : '#007aff', itemTextColor: dark ? '#d1d1d6' : '#48484a',
    },
    DataTable: { thColor: dark ? '#2c2c2e' : '#fafafa', tdColor: surface, tdColorHover: dark ? '#303033' : '#f5f8fc', borderColor: separator, borderRadius: '12px', thFontWeight: '500' },
    Tabs: { tabFontWeightActive: '600' },
  }
}
