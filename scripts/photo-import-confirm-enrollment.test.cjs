const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const root = path.join(__dirname, '..');
const assets = path.join(root, 'internal', 'web', 'assets');
const read = relative => fs.readFileSync(path.join(assets, relative), 'utf8');

test('batch photo import supports direct multi-select images and legacy ZIP uploads', () => {
  const html = read('index.html');
  const script = read('js/photo_import.js');
  const backend = fs.readFileSync(path.join(root, 'internal', 'web', 'photo_import.go'), 'utf8');
  assert.match(html, /id="photoImportFile"[^>]*multiple/);
  assert.match(html, /accept="[^"]*\.jpg[^"]*\.png[^"]*\.zip/);
  assert.match(script, /Array\.from\(input\.files \|\| \[\]\)/);
  assert.match(script, /fd\.append\('files', file, file\.name\)/);
  assert.match(script, /fd\.append\('file', zipFiles\[0\], zipFiles\[0\]\.name\)/);
  assert.match(backend, /r\.MultipartForm\.File\["files"\]/);
  assert.match(backend, /r\.MultipartForm\.File\["file"\]/);
  assert.match(backend, /photoImportIdentity\(displayName\)/);
});

test('FaceSign uses a branded cancel/confirm dialog instead of browser-native prompts', () => {
  const html = read('index.html');
  const core = read('js/core.js');
  assert.match(html, /id="appConfirmModal"/);
  assert.match(html, /id="appConfirmCancel"/);
  assert.match(html, /id="appConfirmAccept"/);
  assert.match(core, /function showConfirmDialog\(message, options = \{\}\)/);
  assert.match(core, /finishConfirmDialog\(false\)/);

  const modules = [
    'attendance.js', 'auth.js', 'boot.js', 'camera.js', 'cameras.js',
    'classes.js', 'core.js', 'debug.js', 'enrollment.js', 'face_compare.js',
    'navigation.js', 'photo_import.js', 'recognition.js', 'seat_editor.js',
    'seating.js', 'settings.js', 'students.js', 'users.js'
  ];
  for (const module of modules) {
    const source = read('js/' + module);
    assert.doesNotMatch(source, /\b(?:window\.)?(?:confirm|alert|prompt)\s*\(/, module + ' must not invoke native dialogs');
  }
});

test('first successful enrollment offers immediate additional-angle capture', () => {
  const html = read('index.html');
  const script = read('js/enrollment.js');
  assert.match(html, /id="enrollmentAngleModal"/);
  for (const angle of ['左侧', '右侧', '轻微抬头', '轻微低头', '其他']) {
    assert.ok(html.includes('data-enrollment-angle="' + angle + '"'), 'missing angle ' + angle);
  }
  const createIndex = script.indexOf("const created = await api('/api/enrollment/create'");
  const panelIndex = script.indexOf('await openSamplesPanel(studentID, {scroll: false})', createIndex);
  const confirmIndex = script.indexOf('await showConfirmDialog(', panelIndex);
  const pickerIndex = script.indexOf('await chooseEnrollmentAngle(', confirmIndex);
  const captureIndex = script.indexOf('await startSupplementAngle(angle)', pickerIndex);
  assert.ok(createIndex >= 0 && panelIndex > createIndex && confirmIndex > panelIndex && pickerIndex > confirmIndex && captureIndex > pickerIndex,
    'after first enrollment, open the sample panel, ask, choose an angle, and start capture in order');
});
