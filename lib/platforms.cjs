'use strict';

const targets = Object.freeze([
  { platform: 'darwin', arch: 'arm64', goos: 'darwin', goarch: 'arm64' },
  { platform: 'darwin', arch: 'x64', goos: 'darwin', goarch: 'amd64' },
  { platform: 'linux', arch: 'arm64', goos: 'linux', goarch: 'arm64' },
  { platform: 'linux', arch: 'x64', goos: 'linux', goarch: 'amd64' },
  { platform: 'win32', arch: 'arm64', goos: 'windows', goarch: 'arm64' },
  { platform: 'win32', arch: 'x64', goos: 'windows', goarch: 'amd64' },
]);

function targetFor(platform = process.platform, arch = process.arch) {
  const target = targets.find(item => item.platform === platform && item.arch === arch);
  if (!target) {
    throw new Error(`Rian does not support ${platform}/${arch}. Use macOS, Linux, or Windows on x64 or arm64.`);
  }
  return { ...target, key: `${platform}-${arch}`, filename: platform === 'win32' ? 'rian.exe' : 'rian' };
}

module.exports = { targets, targetFor };
