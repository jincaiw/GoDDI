// Shared configuration for the GoDDI browser regression suites.
//
// Every value is overridable from the environment so this harness is not tied
// to one developer's machine. Run against an isolated server only; these suites
// create and delete test rows in the configured database.
const fs = require('fs');
const path = require('path');
const os = require('os');

// Fail with an actionable message rather than a bare MODULE_NOT_FOUND when a
// fresh checkout has not installed the harness dependencies yet.
try {
  require.resolve('playwright');
} catch (e) {
  console.error(
    'playwright is not resolvable. Install it for this harness:\n' +
    '  cd test/browser && npm install\n' +
    'or point NODE_PATH at an existing playwright installation.\n' +
    'See test/browser/README.md.'
  );
  process.exit(2);
}

const BASE = process.env.GODDI_BASE_URL || 'http://127.0.0.1:18080';
const USER = process.env.GODDI_ADMIN_USER || 'admin';
const PASS = process.env.GODDI_ADMIN_PASS || '';
const SHOT_DIR = process.env.GODDI_SHOT_DIR || path.join(os.tmpdir(), 'goddi-browser-shots');

// Resolve a Chromium/Chrome executable. Honour an explicit path first, then
// common Playwright cache layouts, then system browsers. Returning undefined
// lets Playwright use its own bundled browser when installed.
function chromiumPath() {
  if (process.env.GODDI_CHROMIUM) return process.env.GODDI_CHROMIUM;
  const home = os.homedir();
  const candidates = [
    path.join(home, 'Library/Caches/ms-playwright'),
    path.join(home, '.cache/ms-playwright'),
  ];
  for (const root of candidates) {
    if (!fs.existsSync(root)) continue;
    for (const dir of fs.readdirSync(root).sort().reverse()) {
      if (!dir.startsWith('chromium')) continue;
      const exe = path.join(root, dir, 'chrome-headless-shell-mac-arm64/chrome-headless-shell');
      const linux = path.join(root, dir, 'chrome-linux/headless_shell');
      const win = path.join(root, dir, 'chrome-win/headless_shell.exe');
      for (const p of [exe, linux, win]) {
        if (fs.existsSync(p)) return p;
      }
    }
  }
  for (const p of [
    '/usr/bin/chromium',
    '/usr/bin/google-chrome',
    '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  ]) {
    if (fs.existsSync(p)) return p;
  }
  return undefined; // Playwright's own managed browser
}

function shot(name) {
  fs.mkdirSync(SHOT_DIR, { recursive: true });
  return path.join(SHOT_DIR, name);
}

function launchOpts() {
  const opts = { headless: true, args: ['--no-sandbox'] };
  const exe = chromiumPath();
  if (exe) opts.executablePath = exe;
  return opts;
}

module.exports = { BASE, USER, PASS, SHOT_DIR, EXEC: chromiumPath(), shot, launchOpts };
