'use strict';

const fs = require('node:fs');
const path = require('node:path');
const { spawn } = require('node:child_process');
const { targetFor } = require('./platforms.cjs');
const { name: packageName } = require('../package.json');

function binaryPath(root, platform = process.platform, arch = process.arch) {
  const target = targetFor(platform, arch);
  const binary = path.join(root, 'dist', 'binaries', target.key, target.filename);
  if (!fs.existsSync(binary) || !fs.statSync(binary).isFile()) {
    throw new Error(`The Rian executable is missing. Reinstall with: npm install -g ${packageName}`);
  }
  if (platform !== 'win32' && !(fs.statSync(binary).mode & 0o111)) {
    try { fs.chmodSync(binary, 0o755); } catch {
      throw new Error(`The Rian executable needs permission to run. Repair the installation with: npm rebuild -g ${packageName}`);
    }
  }
  return binary;
}

function launch(root, args = process.argv.slice(2), spawnProcess = spawn) {
  const binary = binaryPath(root);
  // Native stdio must pass straight through: Bubble Tea owns the terminal.
  const child = spawnProcess(binary, args, { stdio: 'inherit', shell: false, windowsHide: false });
  const handlers = new Map();
  for (const signal of ['SIGINT', 'SIGTERM', ...(process.platform === 'win32' ? [] : ['SIGHUP'])]) {
    const handler = () => { if (child.exitCode === null && child.signalCode === null) child.kill(signal); };
    handlers.set(signal, handler);
    process.on(signal, handler);
  }
  const cleanup = () => { for (const [signal, handler] of handlers) process.removeListener(signal, handler); };
  child.once('error', err => {
    cleanup();
    console.error(`rian: could not start the executable: ${err.message}`);
    process.exitCode = 1;
  });
  child.once('exit', (code, signal) => {
    cleanup();
    process.exitCode = code ?? ({ SIGINT: 130, SIGTERM: 143, SIGHUP: 129 }[signal] ?? 1);
  });
  return child;
}

module.exports = { binaryPath, launch };
