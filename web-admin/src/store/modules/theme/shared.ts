import { defu } from 'defu';
import { getColorPalette, getRgb } from '@sa/color';
import { DARK_CLASS } from '@/constants/app';
import { toggleHtmlClass } from '@/utils/common';
import { localStg } from '@/utils/storage';
import { overrideThemeSettings, themeSettings } from '@/theme/settings';
import { themeVars } from '@/theme/vars';

/** Init theme settings */
export function initThemeSettings() {
  const isProd = import.meta.env.PROD;

  // if it is development mode, the theme settings will not be cached, by update `themeSettings` in `src/theme/settings.ts` to update theme settings
  if (!isProd) return themeSettings;

  // if it is production mode, the theme settings will be cached in localStorage
  // if want to update theme settings when publish new version, please update `overrideThemeSettings` in `src/theme/settings.ts`

  const localSettings = localStg.get('themeSettings');

  let settings = defu(localSettings, themeSettings);

  const isOverride = localStg.get('overrideThemeFlag') === BUILD_TIME;

  if (!isOverride) {
    settings = defu(overrideThemeSettings, settings);

    localStg.set('overrideThemeFlag', BUILD_TIME);
  }

  return settings;
}

/**
 * create theme token css vars value by theme settings
 *
 * @param colors Theme colors
 * @param tokens Theme setting tokens
 * @param [recommended=false] Use recommended color. Default is `false`
 */
export function createThemeToken(
  colors: App.Theme.ThemeColor,
  tokens?: App.Theme.ThemeSetting['tokens'],
  recommended = false
) {
  const paletteColors = createThemePaletteColors(colors, recommended);

  const { light, dark } = tokens || themeSettings.tokens;

  const themeTokens: App.Theme.ThemeTokenCSSVars = {
    colors: {
      ...paletteColors,
      nprogress: paletteColors.primary,
      ...light.colors
    },
    boxShadow: {
      ...light.boxShadow
    }
  };

  const darkThemeTokens: App.Theme.ThemeTokenCSSVars = {
    colors: {
      ...themeTokens.colors,
      ...dark?.colors
    },
    boxShadow: {
      ...themeTokens.boxShadow,
      ...dark?.boxShadow
    }
  };

  return {
    themeTokens,
    darkThemeTokens
  };
}

/**
 * Create theme palette colors
 *
 * @param colors Theme colors
 * @param [recommended=false] Use recommended color. Default is `false`
 */
function createThemePaletteColors(colors: App.Theme.ThemeColor, recommended = false) {
  const colorKeys = Object.keys(colors) as App.Theme.ThemeColorKey[];
  const colorPaletteVar = {} as App.Theme.ThemePaletteColor;

  colorKeys.forEach(key => {
    const colorMap = getColorPalette(colors[key], recommended);

    colorPaletteVar[key] = colorMap.get(500)!;

    colorMap.forEach((hex, number) => {
      colorPaletteVar[`${key}-${number}`] = hex;
    });
  });

  return colorPaletteVar;
}

/**
 * Get css var by tokens
 *
 * @param tokens Theme base tokens
 */
function getCssVarByTokens(tokens: App.Theme.BaseToken) {
  const styles: string[] = [];

  function removeVarPrefix(value: string) {
    return value.replace('var(', '').replace(')', '');
  }

  function removeRgbPrefix(value: string) {
    return value.replace('rgb(', '').replace(')', '');
  }

  for (const [key, tokenValues] of Object.entries(themeVars)) {
    for (const [tokenKey, tokenValue] of Object.entries(tokenValues)) {
      let cssVarsKey = removeVarPrefix(tokenValue);
      let cssValue = tokens[key][tokenKey];

      if (key === 'colors') {
        cssVarsKey = removeRgbPrefix(cssVarsKey);
        const { r, g, b } = getRgb(cssValue);
        cssValue = `${r} ${g} ${b}`;
      }

      styles.push(`${cssVarsKey}: ${cssValue}`);
    }
  }

  const styleStr = styles.join(';');

  return styleStr;
}

/**
 * Bridge generated palette tokens to TDesign's `--td-*` variables.
 *
 * TDesign components read `--td-*`, while the theme store generates
 * `--primary-*` / status-palette variables. Writing the bridge here makes
 * themeColor, otherColor and themeRadius authoritative in both schemes. The
 * runtime stylesheet is appended after the bundled CSS, so
 * equal-specificity overrides win.
 */
function createTDesignBridge(colors: Record<string, string>, isDark: boolean, radiusOverride?: number) {
  const declarations: string[] = [];

  function shade(key: string, number: number, fallback: string) {
    return colors[`${key}-${number}`] || fallback;
  }

  function push(name: string, value?: string) {
    if (value) declarations.push(`${name}: ${value}`);
  }

  const semanticColors: [string, string][] = [
    ['brand', 'primary'],
    ['warning', 'warning'],
    ['error', 'error'],
    ['success', 'success']
  ];

  for (const [tdKey, themeKey] of semanticColors) {
    const baseColor = colors[themeKey];
    if (!baseColor) continue;

    // Two different needs in dark mode:
    //  - brand TEXT on a dark card needs a light tone (base #0052D9 = 2.8:1);
    //  - a FILLED primary button keeps white text, so its background must stay
    //    dark enough for 4.5:1 (a light 300 tone drops to 2.96:1).
    // Keep the base colour as the fill and expose the light tone as a separate
    // variable consumed by text-only usages.
    const surfaceColor = isDark ? baseColor : baseColor;
    const textBrandColor = isDark ? (colors[`${themeKey}-300`] || colors[`${themeKey}-200`] || baseColor) : baseColor;
    push(`--td-${tdKey}-color`, surfaceColor);
    push(`--td-${tdKey}-color-hover`, shade(themeKey, 400, baseColor));
    push(`--td-${tdKey}-color-active`, shade(themeKey, 600, baseColor));
    push(`--td-${tdKey}-color-focus`, shade(themeKey, 200, baseColor));
    push(`--td-${tdKey}-color-disabled`, shade(themeKey, 300, baseColor));
    push(`--td-${tdKey}-color-light`, shade(themeKey, isDark ? 900 : 50, baseColor));
    push(`--td-${tdKey}-color-light-hover`, shade(themeKey, isDark ? 800 : 100, baseColor));

    if (tdKey === 'brand') {
      push('--td-text-color-brand', textBrandColor);
      push('--td-text-color-link', isDark ? textBrandColor : shade(themeKey, 600, baseColor));
    }

    // Text-only uses of the semantic colours (TLink, helper text) must clear
    // 4.5:1 on the container background. The 500-level palette tones reach only
    // 3.84:1 (danger) and 2.81:1 (warning) on white, so expose a darker
    // text-safe tone for those two.
    if (tdKey === 'error' || tdKey === 'warning') {
      push(`--td-${tdKey}-color-text`, isDark ? textBrandColor : shade(themeKey, 700, baseColor));
    }
  }

  // Read the runtime radius so the drawer control takes effect immediately;
  // fall back to the compiled default when it is not provided.
  const radius = Number(radiusOverride ?? themeSettings.themeRadius) || 6;
  push('--td-radius-small', `${Math.max(1, radius - 3)}px`);
  push('--td-radius-default', `${radius}px`);
  push('--td-radius-medium', `${radius}px`);
  push('--td-radius-large', `${radius + 3}px`);

  return declarations.join(';');
}

/**
 * Add theme vars to global
 *
 * @param tokens
 */
export function addThemeVarsToGlobal(tokens: App.Theme.BaseToken, darkTokens: App.Theme.BaseToken, runtimeRadius?: number) {
  const cssVarStr = getCssVarByTokens(tokens);
  const darkCssVarStr = getCssVarByTokens(darkTokens);
  const bridgeVars = createTDesignBridge(tokens.colors, false, runtimeRadius);
  const darkBridgeVars = createTDesignBridge(darkTokens.colors, true, runtimeRadius);

  const css = `
    :root {
      ${cssVarStr}; ${bridgeVars}
    }
  `;

  // Match TDesign's own dark selector specificity (:root.dark); `html.dark`
  // has lower specificity (0,1,1) and would lose to it regardless of order.
  const darkCss = `
    :root.${DARK_CLASS} {
      ${darkCssVarStr}; ${darkBridgeVars}
    }
  `;

  const styleId = 'theme-vars';

  const style = document.querySelector(`#${styleId}`) || document.createElement('style');

  style.id = styleId;

  style.textContent = css + darkCss;

  document.head.appendChild(style);
}

/**
 * Toggle css dark mode
 *
 * @param darkMode Is dark mode
 */
export function toggleCssDarkMode(darkMode = false) {
  const { add, remove } = toggleHtmlClass(DARK_CLASS);

  if (darkMode) {
    add();
  } else {
    remove();
  }
}
/**
 * Toggle auxiliary color modes
 *
 * @param grayscaleMode
 * @param colourWeakness
 */
export function toggleAuxiliaryColorModes(grayscaleMode = false, colourWeakness = false) {
  const target = document.getElementById('app');
  if (!target) return;

  // Avoid filter on <html>: it creates a containing block for fixed dialogs
  // and inverts semantic status colors. The app root limits the effect and
  // grayscale remains a safe low-vision aid. Colour weakness uses a mild
  // saturation reduction instead of a destructive invert.
  target.style.filter = [grayscaleMode ? 'grayscale(100%)' : '', colourWeakness ? 'saturate(0.75)' : '']
    .filter(Boolean)
    .join(' ');
}
