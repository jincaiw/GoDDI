/** Default theme settings */
export const themeSettings: App.Theme.ThemeSetting = {
  themeScheme: 'light',
  grayscale: false,
  colourWeakness: false,
  recommendColor: false,
  // TDesign Vue Next Starter dashboard/base default theme ('devflow').
  themeColor: '#0052D9',
  themeRadius: 6,
  otherColor: {
    info: '#2080f0',
    success: '#00A870',
    warning: '#ED7B2F',
    error: '#E34D59'
  },
  isInfoFollowPrimary: false,
  layout: {
    mode: 'vertical',
    scrollMode: 'content'
  },
  page: {
    animate: true,
    animateMode: 'fade-slide'
  },
  header: {
    height: 56,
    breadcrumb: {
      visible: false,
      showIcon: true
    },
    multilingual: {
      visible: true
    },
    globalSearch: {
      visible: true
    }
  },
  tab: {
    visible: false,
    cache: true,
    height: 44,
    mode: 'chrome',
    closeTabByMiddleClick: false
  },
  fixedHeaderAndTab: true,
  sider: {
    inverted: false,
    width: 220,
    collapsedWidth: 64,
    mixWidth: 90,
    mixCollapsedWidth: 64,
    mixChildMenuWidth: 200,
    autoSelectFirstMenu: false
  },
  footer: {
    visible: true,
    fixed: false,
    height: 48,
    right: true
  },
  watermark: {
    visible: false,
    text: 'GoDDI',
    enableUserName: false,
    enableTime: false,
    timeFormat: 'YYYY-MM-DD HH:mm'
  },
  tokens: {
    light: {
      colors: {
        container: 'rgb(255, 255, 255)',
        layout: 'rgb(243, 244, 246)',
        inverted: 'rgb(0, 31, 151)',
        'base-text': 'rgb(31, 31, 31)'
      },
      boxShadow: {
        header: '0 1px 2px rgb(0, 21, 41, 0.08)',
        sider: '2px 0 8px 0 rgb(29, 35, 41, 0.05)',
        tab: '0 1px 2px rgb(0, 21, 41, 0.08)'
      }
    },
    dark: {
      colors: {
        container: 'rgb(36, 36, 36)',
        layout: 'rgb(17, 17, 17)',
        'base-text': 'rgb(224, 224, 224)'
      }
    }
  }
};

/**
 * Override theme settings
 *
 * If publish new version, use `overrideThemeSettings` to override certain theme settings
 */
export const overrideThemeSettings: Partial<App.Theme.ThemeSetting> = {
  themeScheme: 'light',
  themeColor: '#0052D9',
  themeRadius: 6,
  otherColor: {
    info: '#2080f0',
    success: '#00A870',
    warning: '#ED7B2F',
    error: '#E34D59'
  },
  isInfoFollowPrimary: false,
  layout: { mode: 'vertical', scrollMode: 'content' },
  header: {
    height: 56,
    breadcrumb: { visible: false, showIcon: true },
    multilingual: { visible: true },
    globalSearch: { visible: true }
  },
  tab: {
    visible: false,
    cache: true,
    height: 44,
    mode: 'chrome',
    closeTabByMiddleClick: false
  }
};