import type { App } from 'vue';
// Component styles only (no global reset) so not-yet-migrated Naive UI pages
// keep their own styling during the staged migration.
import 'tdesign-vue-next/dist/tdesign.css';
import TDesign from 'tdesign-vue-next';
import TDesignIcons from 'tdesign-icons-vue-next';

/**
 * Registers TDesign Vue Next globally.
 *
 * The console is being migrated from Naive UI to TDesign component-by-component
 * (see docs/tdesign-migration.md). Both libraries are registered during the
 * transition so that migrated pages use TDesign and not-yet-migrated pages keep
 * working; the Naive UI registration is removed once the last page moves over.
 */
export function setupTDesign(app: App) {
  app.use(TDesign);
  app.use(TDesignIcons);
}
