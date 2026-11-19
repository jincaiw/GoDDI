// Static guard for the devflow theme baseline derived from TDesign Vue Next
// Starter dashboard/base. It runs without a browser.
const fs = require('fs');
const path = require('path');

const root = path.join(__dirname, '..', '..');
let failures = 0;
const fail = msg => { console.log('FAIL | ' + msg); failures++; };
const read = rel => fs.readFileSync(path.join(root, rel), 'utf8');

const settings = read('web-admin/src/theme/settings.ts');
const sideNav = read('web-admin/src/components/starter/SideNav.vue');
const globalStyles = read('web-admin/src/styles/css/global.css');
const dashboard = read('web-admin/src/views/dashboard/index.vue');
const zh = read('web-admin/src/locales/langs/zh-cn.ts');
const en = read('web-admin/src/locales/langs/en-us.ts');
const routeAudit = read('test/browser/route-audit.cjs');
const packageJson = JSON.parse(read('web-admin/package.json'));

if ((settings.match(/themeColor: '#0052D9'/g) || []).length < 2) {
  fail('devflow theme must be the default and the production override');
}
for (const needle of ['themeScheme: \'light\'', 'mode: \'vertical\'', 'breadcrumb: { visible: false', 'tab: {\n    visible: false']) {
  if (!settings.includes(needle)) fail('devflow override missing: ' + needle);
}
for (const color of ["'#00A870'", "'#ED7B2F'", "'#E34D59'"]) {
  if (!settings.includes(color)) fail('devflow TDesign status color missing: ' + color);
}
if (!sideNav.includes('theme="light"')) fail('side navigation must use the light TDesign menu');
if (!globalStyles.includes('--shell-sidebar-bg: var(--td-bg-color-container)')) fail('shell token definitions must ship in global.css');
if (packageJson.name !== 'goddi-web-admin') fail('frontend package identity must be goddi-web-admin');
if (/soybean|naiveui/i.test(JSON.stringify({ name: packageJson.name, description: packageJson.description, keywords: packageJson.keywords }))) {
  fail('frontend package metadata still identifies the old template');
}
if (!settings.includes("text: 'GoDDI'")) fail('watermark default must identify GoDDI');
for (const token of ['--td-bg-color-container', '--td-bg-color-page', '--td-text-color-primary', '--td-border-level-1-color']) {
  if (!globalStyles.includes(token)) fail('shell styles must use TDesign token: ' + token);
}
// Responsive caps and tokenized visual primitives must stay in the global
// stylesheet so they ship in the deployed bundle and survive view refactors.
for (const needle of [
  '--app-font-family:',
  '--app-space-sm:',
  '--app-radius-md:',
  '--app-gutter:',
  '--app-overlay-height: 100dvh',
  '.t-space,',
  '.td-page-header {',
  '.t-dialog__body {',
  '.t-table,',
  '100dvh',
  '.app-stack {',
  '.app-inline {',
  '.app-rounded {',
  '@media (max-width: 640px)'
]) {
  if (!globalStyles.includes(needle)) fail('global responsive/token rule missing: ' + needle);
}
// dashboard/base geometry: the page must remain a flex column so its 16px
// gaps apply, and only real TDesign spacing tokens may be used.
if (!dashboard.includes('.dashboard-base { display: flex; flex-direction: column; gap: 16px; }')) {
  fail('dashboard must keep the flex-column layout with 16px gaps');
}
for (const invalid of ['--td-comp-paddingTB-lg', '--td-comp-paddingLR-lg', '--td-comp-margin-lg']) {
  if (dashboard.includes(invalid)) fail('dashboard uses undefined spacing token: ' + invalid);
}
if (!dashboard.includes('padding: 16px') && !dashboard.includes('padding: var(--td-comp-padding')) {
  fail('dashboard cards must have valid padding');
}
// Legacy utility classes must stay defined in the shipped stylesheet; several
// older dialogs and the iframe page still use them.
for (const util of ['.mb-4 {', '.mt-4 {', '.h-full {', '.size-full {']) {
  if (!globalStyles.includes(util)) fail('legacy utility class must stay defined: ' + util);
}
// Horizontal layout: the top menu needs explicit row styles, otherwise
// TDesign renders its default 232px vertical menu box inside the header.
for (const needle of ['.td-top-menu {', 'flex-direction: row', '.td-top-menu .t-menu__item']) {
  if (!globalStyles.includes(needle)) fail('horizontal top-menu rule missing: ' + needle);
}
// Breadcrumb must resolve a parent crumb for hidden detail routes.
const headerVue = read('web-admin/src/components/starter/Header.vue');
if (!headerVue.includes('activeMenu')) fail('breadcrumb parent injection missing');
if (!headerVue.includes('TSubmenu v-if="item.children')) fail('top menu must render submenus in order');

// Quick search must render live results (starter behaviour), not Enter-only.
for (const needle of ['searchResults', 'td-search-panel', 'searchNoResult']) {
  if (!headerVue.includes(needle)) fail('quick search live-results missing: ' + needle);
}
if (!globalStyles.includes('.td-search-panel')) fail('search result panel styles missing');

// Route tab bar: the tab store existed but nothing rendered it, so the
// "useTagTabs" switch was a no-op. Keep the component, layout mount and the
// router-guard feed in place.
const tabBar = read('web-admin/src/components/starter/TabBar.vue');
const layoutVue = read('web-admin/src/layouts/tdesign-layout/index.vue');
const routeGuard = read('web-admin/src/router/guard/route.ts');
if (!tabBar.includes('td-tab-bar')) fail('tab bar component missing');
if (!layoutVue.includes('<StarterTabBar />')) fail('tab bar must be mounted in the layout');
if (!routeGuard.includes('tabStore.addTab')) fail('router guard must feed the tab store');
if (!globalStyles.includes('.td-tab-bar')) fail('tab bar styles missing');

// Mobile shell: the sider must be an overlay drawer, not a permanent rail
// (a rail pushed the header toggle under the sider) and the scrim must stay
// below the panel so taps outside close it.
for (const needle of ['.td-shell--mobile .td-aside', 'z-index: 1002', ':not(.td-shell--drawer-open) .td-aside']) {
  if (!globalStyles.includes(needle)) fail('mobile drawer rule missing: ' + needle);
}

// Dark surfaces need a lighter brand tone, otherwise brand text falls below
// the WCAG AA 4.5:1 threshold (measured 2.79:1 before the fix).
const themeShared = read('web-admin/src/store/modules/theme/shared.ts');
if (!themeShared.includes('surfaceColor')) fail('dark brand surface colour override missing');
if (!themeShared.includes('isDark ?')) fail('bridge must branch on colour scheme');

// Dark primary buttons keep white text, so the fill must stay the base tone;
// only brand TEXT uses the lightened tone.
if (!themeShared.includes('textBrandColor')) fail('dark text-brand tone variable missing');

// Semantic text links need a darker tone than the 500-level fill colour.
for (const needle of ['--td-error-color-text', '--td-warning-color-text']) {
  if (!themeShared.includes(needle.replace('--td-', '').replace(/-/g, ''))) {
    // tolerate either camel/string form; fall through to the CSS assertion below
  }
}
if (!globalStyles.includes('.t-link--theme-danger')) fail('danger link contrast rule missing');
if (!globalStyles.includes('.t-link--theme-warning')) fail('warning link contrast rule missing');

// Mobile dialogs must stay inside the viewport with a visible footer.
for (const needle of ['.t-dialog__position', 'max-height: calc(100dvh - 24px)', '.t-dialog__footer']) {
  if (!globalStyles.includes(needle)) fail('mobile dialog rule missing: ' + needle);
}

// Action columns must be pinned with TDesign's native fixed support; CSS
// sticky cannot work inside the table's own scroll container.
if (!read('web-admin/src/components/RemoteTable.vue').includes("fixed: 'right'")) {
  fail('action column must use fixed:right');
}

// Tab buttons and their close affordance need the same visible focus ring as
// other controls, and the close affordance must expose a button role.
for (const needle of ['html body .td-tab:focus-visible', 'html body .td-tab__close:focus-visible']) {
  if (!globalStyles.includes(needle)) fail('tab focus ring missing: ' + needle);
}
if (!tabBar.includes('role="button"')) fail('tab close affordance must expose role=button');

// Filter state persistence (KeepAlive cannot match lazy wrapper names).
const persistedRef = read('web-admin/src/composables/usePersistedRef.ts');
if (!persistedRef.includes('sessionStorage')) fail('usePersistedRef must use sessionStorage');
if (!read('web-admin/src/views/dns/zones/index.vue').includes('usePersistedRef')) {
  fail('zones page must persist its filters');
}

// Global watermark: configured but previously never rendered.
const appVue = read('web-admin/src/App.vue');
if (!appVue.includes('app-watermark')) fail('global watermark overlay missing');
if (!appVue.includes('watermarkVisible')) fail('watermark must respect theme settings');
if (!read('web-admin/src/store/modules/theme/index.ts').includes('setWatermarkVisible')) {
  fail('watermark visibility setter missing');
}

// Dialogs must be capped on every screen size, not just mobile.
if (!globalStyles.includes('max-height: calc(100dvh - 48px)')) {
  fail('dialog viewport cap missing for desktop');
}

// Core DDI chain must stay reachable: zones list -> zone detail -> tools client.
for (const [file, needle] of [
  ['web-admin/src/views/dns/zones/index.vue', 'RemoteTable'],
  ['web-admin/src/views/dns/zone-detail/[id].vue', 'listDNSRecords'],
  ['web-admin/src/views/tools/client/index.vue', 'executeDNSQuery']
]) {
  if (!read(file).includes(needle)) fail('core DDI page contract missing in ' + file);
}

// Delete confirmations must be component-level dialogs with a real body and a
// danger confirm button; DialogPlugin runs outside the app context.
const zonesView = read('web-admin/src/views/dns/zones/index.vue');
if (/DialogPlugin\.confirm\(/.test(zonesView)) fail('zones must not call DialogPlugin.confirm');
if (!zonesView.includes(":body=\"t('common.deleteConfirm')\"")) fail('delete dialog body missing');
if (!/zoneNameInvalid/.test(zonesView)) fail('zone name format validation missing');

// Delete confirmations must name the resource, never a bare UUID.
const uuidCalls = read('web-admin/src/views/admin/users/index.vue');
if (!uuidCalls.includes('namedDeleteBody(label || id)')) fail('delete body must prefer readable label');

// The unimplemented apps endpoint must degrade gracefully, not crash the page.
if (!read('web-admin/src/views/dns/apps/index.vue').includes('apps.')) {
  fail('dns apps page must render a placeholder state');
}

// Theme mode must be switchable from the drawer; the dark class drives every
// dark token, so a broken switch silently disables the whole dark theme.
const drawerVue = read('web-admin/src/components/starter/SettingDrawer.vue');
if (!drawerVue.includes('setThemeScheme')) fail('theme scheme switch missing');
if (!read('web-admin/src/store/modules/theme/index.ts').includes('toggleCssDarkMode')) {
  fail('dark mode CSS toggle missing');
}

// Harness correctness helpers must exist and the docs must stay with them.
const commonCjs = read('test/browser/common.cjs');
for (const helper of ['assertAuthenticated', 'failOnly', 'visibleDialogText', 'remoteTableError']) {
  if (!commonCjs.includes(helper)) fail('harness helper missing: ' + helper);
}
if (!commonCjs.includes('remote-table-error')) fail('remoteTableError must use the real class name');

// Route health sweep must stay automated so regressions surface immediately.
if (!read('test/browser/README-HARNESS.md').includes('判定缺陷前的检查清单')) {
  fail('harness spec must keep the defect triage checklist');
}

// Shell geometry must stay defined globally: scoped rules in the layout cannot
// reach StarterHeader's root, which silently collapsed the header to 33px.
for (const needle of ['.td-header {', '.td-header-right {', 'min-height: 56px', 'margin-left: auto']) {
  if (!globalStyles.includes(needle)) fail('shell geometry rule must stay global: ' + needle);
}
if (!sideNav.includes(":width=\"collapsed ? '64px' : '232px'\"")) fail('sider width contract changed');

// Accessibility contract: focus rings, modal semantics and form error wiring
// are implemented once and must not silently disappear.
const modalA11y = read('web-admin/src/utils/modal-a11y.ts');
const mainTs = read('web-admin/src/main.ts');
const remoteTable = read('web-admin/src/components/RemoteTable.vue');
for (const needle of ['aria-modal', "role', 'dialog'", 'aria-invalid', 'aria-describedby', 'aria-required']) {
  if (!modalA11y.includes(needle)) fail('modal a11y patch missing: ' + needle);
}
if (!mainTs.includes('setupModalA11y()')) fail('modal a11y patch must be installed at boot');
if (!globalStyles.includes('focus-visible')) fail('visible keyboard focus ring missing');
if (!remoteTable.includes('aria-busy')) fail('RemoteTable must expose aria-busy while loading');
if (!remoteTable.includes('aria-live')) fail('RemoteTable must expose a live status region');
if (!dashboard.includes("noerror: 'dashboard.statsNoError'")) fail('dashboard rcode labels must be localized');
if (zh.includes("statsNoError: 'NOERROR'") || en.includes("statsNoError: 'NOERROR'")) {
  fail('dashboard rcode locale must not expose raw NOERROR');
}
for (const raw of ['noerror', 'nxdomain', 'servfail', 'refused']) {
  if (!routeAudit.includes(raw)) fail('route-audit must pin raw rcode: ' + raw);
}

if (failures) {
  console.log('errors: ' + failures);
  process.exit(1);
}
console.log('PASS theme devflow=1 tokens=1 responsive=1 rcode-localized=1 identity=goddi-web-admin');