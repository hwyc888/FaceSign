const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const root = path.join(__dirname, '..');
const offlineDir = path.join(root, 'scripts', 'ubuntu', 'offline');
const read = name => fs.readFileSync(path.join(offlineDir, name), 'utf8').replace(/\r\n/g, '\n');

test('offline installer targets Ubuntu 24.04 AMD64 and installs only from local DEBs', () => {
  const script = read('install.sh');
  assert.match(script, /VERSION_ID.*24\.04/);
  assert.match(script, /print-architecture.*amd64/);
  assert.match(script, /apt-get install --no-download/);
  assert.match(script, /runtime\/facesign/);
  assert.match(script, /libonnxruntime\.so\.1\.26\.0/);
  assert.match(script, /command -v ffmpeg/);
  assert.match(script, /command -v ffprobe/);
  assert.match(script, /LD_LIBRARY_PATH=/);
  assert.match(script, /sha256sum -c SHA256SUMS/);
  assert.match(script, /sha256sum -c MANIFEST\\.sha256/);
  assert.match(script, /det_10g\.onnx/);
  assert.match(script, /w600k_r50\.onnx/);
  assert.match(script, /FACESIGN_AUTO_START/);
  assert.match(script, /start\.sh/);
});

test('offline DEB preparation downloads the Ubuntu 24.04 dependency set in Docker', () => {
  const script = read('prepare-debs.sh');
  assert.match(script, /docker run/);
  assert.match(script, /ubuntu:24\.04/);
  assert.match(script, /--platform linux\/amd64/);
  assert.match(script, /--download-only/);
  for (const name of ['ca-certificates', 'curl', 'ffmpeg', 'libstdc\+\+6', 'libgomp1', 'libgcc-s1']) {
    assert.ok(script.includes(name), 'missing offline package ' + name);
  }
  assert.match(script, /sha256sum \*\.deb/);
});

test('local full-bundle builder requires explicit model redistribution authorization', () => {
  const script = read('build-full-offline-bundle.sh');
  assert.match(script, /FACESIGN_CONFIRM_MODEL_LICENSE=YES/);
  assert.match(script, /MODEL_LICENSE\.txt/);
  assert.match(script, /scrfd_10g\.onnx/);
  assert.match(script, /arcface_w600k_r50\.onnx/);
  assert.match(script, /SHA256SUMS/);
  assert.match(script, /--exclude='\*\.db'/);
  assert.match(script, /Full-Offline\.tar\.gz/);
  assert.match(script, /MANIFEST\\.sha256/);
});

test('Ubuntu CI publishes a separate offline runtime artifact without embedding restricted identity weights', () => {
  const workflow = fs.readFileSync(path.join(root, '.github', 'workflows', 'build-ubuntu.yml'), 'utf8');
  assert.match(workflow, /facesign-ubuntu-amd64-offline-runtime/);
  assert.match(workflow, /docker run/);
  assert.match(workflow, /scripts\/ubuntu\/offline\/install\.sh/);
  assert.match(workflow, /scripts\/ubuntu\/offline\/build-full-offline-bundle\.sh/);
  assert.match(workflow, /ubuntu-offline-package\.test\.cjs/);
});
