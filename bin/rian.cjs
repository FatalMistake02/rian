#!/usr/bin/env node
'use strict';

const path = require('node:path');
const { launch } = require('../lib/launcher.cjs');

try {
  launch(path.resolve(__dirname, '..'));
} catch (err) {
  console.error(`rian: ${err.message}`);
  process.exitCode = 1;
}
