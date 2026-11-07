'use strict';

const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');
const { checkPackage } = require('./check-package.cjs');
const { binaryPath } = require('../lib/launcher.cjs');
const pkg = require('../package.json');

const root = path.resolve(__dirname, '..');

function shellQuote(value) {
  return "'" + value.replaceAll("'", "'\\''") + "'";
}

function checkDesktopSetup(installed, temporary, env, version) {
  if (process.platform !== 'linux') return;
  const probe = spawnSync('script', ['--version'], { env, encoding: 'utf8', timeout: 5_000 });
  if (probe.error || probe.status !== 0 || !probe.stdout.includes('util-linux')) {
    throw new Error('Desktop setup verification on Linux requires util-linux script');
  }
  const dataHome = path.join(temporary, 'desktop-data');
  const setupEnv = {
    ...env,
    RIAN_CONFIG_HOME: path.join(temporary, 'desktop-config'),
    XDG_DATA_HOME: dataHome,
    SHELL: '/bin/sh',
  };
  const setup = path.join(installed, 'scripts', 'ensure-binary.cjs');
  const command = `exec ${shellQuote(process.execPath)} ${shellQuote(setup)}`;
  const result = spawnSync('script', ['--quiet', '--return', '--command', command, '/dev/null'], {
    cwd: temporary,
    env: setupEnv,
    input: 'yes\n',
    encoding: 'utf8',
    timeout: 30_000,
  });
  if (result.error || result.status !== 0 || !result.stdout.includes('Rian added to your apps.')) {
    throw new Error(`The installed setup did not register the desktop launcher: ${result.error?.message || result.stderr || result.stdout}`);
  }
  const desktop = fs.readFileSync(path.join(dataHome, 'applications', 'rian.desktop'), 'utf8');
  const binary = fs.realpathSync(binaryPath(installed));
  const exec = desktop.split(/\r?\n/).find(line => line.startsWith('Exec='));
  if (exec !== `Exec="${binary}"` || !/^Terminal=true$/m.test(desktop)) {
    throw new Error('The setup-added desktop launcher must open the installed native binary in a terminal');
  }
  const emptyPath = path.join(temporary, 'empty-path');
  fs.mkdirSync(emptyPath);
  const launched = spawnSync(binary, ['--version'], {
    cwd: temporary,
    env: { ...setupEnv, PATH: emptyPath },
    encoding: 'utf8',
    timeout: 10_000,
  });
  if (launched.error || launched.status !== 0 || launched.stdout.trim() !== `rian ${version}`) {
    throw new Error('The setup-added desktop launcher cannot run without Node.js on PATH');
  }
  console.log('Linux setup-added desktop launcher verified without Node.js on PATH.');
}

function checkInstall(tarball) {
  if (!process.env.npm_execpath) throw new Error('Run this check with npm run test:install');
  const temporary = fs.mkdtempSync(path.join(os.tmpdir(), 'rian install-'));
  const config = path.join(temporary, 'config');
  fs.mkdirSync(config);
  // Exercise postinstall without offering a desktop launcher on the developer's terminal.
  fs.writeFileSync(path.join(config, 'app-offer.json'), '{"asked":true}\n');
  const env = { ...process.env, RIAN_CONFIG_HOME: config };
  const run = (command, args, cwd = root) => {
    const result = spawnSync(command, args, { cwd, env, encoding: 'utf8', windowsHide: true });
    if (result.error || result.status !== 0) {
      throw new Error(result.error?.message || result.stderr?.trim() || result.stdout?.trim() || `Command failed (${result.status})`);
    }
    return result.stdout;
  };
  const npm = args => run(process.execPath, [process.env.npm_execpath, ...args]);
  try {
    if (tarball) {
      tarball = path.resolve(tarball);
    } else {
      checkPackage();
      const packed = JSON.parse(npm(['pack', '--ignore-scripts', '--json', '--pack-destination', temporary]));
      tarball = path.join(temporary, packed[0].filename);
    }
    const installation = path.join(temporary, 'installation');
    fs.mkdirSync(installation);
    // Newer npm versions require project-scoped lifecycle allowances here.
    // Older versions ignore the metadata and run scripts by default.
    fs.writeFileSync(path.join(installation, 'package.json'), JSON.stringify({
      private: true,
      allowScripts: { [pkg.name]: true },
    }) + '\n');
    const installationOutput = npm(['install', '--prefix', installation, '--offline', '--no-audit', '--no-fund', '--no-save', '--no-package-lock', '--ignore-scripts=false', '--foreground-scripts', '--loglevel=notice', tarball]);
    if (!installationOutput.includes('node scripts/ensure-binary.cjs')) {
      throw new Error('npm did not execute the package postinstall script');
    }
    const installed = path.join(installation, 'node_modules', pkg.name);
    const manifest = checkPackage(installed);
    const version = run(process.execPath, [path.join(installed, 'bin', 'rian.cjs'), '--version'], temporary).trim();
    if (version !== `rian ${manifest.version}`) throw new Error('The installed npm launcher returned an incorrect version');
    checkDesktopSetup(installed, temporary, env, manifest.version);
    console.log(`Rian ${manifest.version}: packed installation, postinstall, and launcher verified.`);
  } finally {
    fs.rmSync(temporary, { recursive: true, force: true });
  }
}

try { checkInstall(process.argv[2]); }
catch (err) { console.error(`rian install check: ${err.message}`); process.exitCode = 1; }
