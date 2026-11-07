'use strict';

// Repair permissions and offer a per-user app launcher once. No downloads or builds.
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const { binaryPath } = require('../lib/launcher.cjs');

try {
  const binary = binaryPath(path.resolve(__dirname, '..'));
  const result = spawnSync(binary, ['--install-app'], { stdio: 'inherit', shell: false, windowsHide: false });
  // Optional app registration must not fail an otherwise valid installation.
  if (result.error) console.error(`rian: app setup unavailable: ${result.error.message}`);
} catch (err) {
  console.error(`rian: ${err.message}`);
  process.exitCode = 1;
}
