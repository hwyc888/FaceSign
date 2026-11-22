const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const root = path.join(__dirname, '..');
const assets = path.join(root, 'internal', 'web', 'assets');
const read = relative => fs.readFileSync(path.join(assets, relative), 'utf8');

test('batch photo import supports default class, six angles, direct multi-select, and ZIP uploads', () => {
  const html = read('index.html');
  const script = read('js/photo_import.js').replace(/\r\n/g, '\n');
  const backend = fs.readFileSync(path.join(root, 'internal', 'web', 'photo_import.go'), 'utf8');
  assert.match(html, /id="photoImportFile"[^>]*multiple/);
  assert.match(html, /id="photoImportDefaultClass"/);
  assert.match(html, /id="photoImportDefaultAngle"/);
  assert.match(html, /id="photoImportBulkAngle"/);
  assert.match(html, /id="applyPhotoImportAngle"/);
  assert.match(html, /accept="[^"]*\.jpg[^"]*\.png[^"]*\.zip/);
  for (const angle of ['正面', '左侧', '右侧', '轻微抬头', '轻微低头', '其他']) {
    assert.ok(html.includes('value="' + angle + '"'), 'missing import angle ' + angle);
  }
  assert.match(html, /ZIP 内与系统班级同名的目录优先按目录识别/);
  assert.match(html, /左右方向、抬头\/低头方向请在预览中人工确认/);
  assert.match(script, /Array\.from\(input\.files \|\| \[\]\)/);
  assert.match(script, /fd\.append\('class_name', defaultClassName\)/);
  assert.match(script, /photoImportDefaultClass/);
  assert.match(script, /fd\.append\('angle', defaultAngle\)/);
  assert.match(script, /photoImportAngleSelect\(item\)/);
  assert.match(script, /fd\.append\('class_name', defaultClassName\)/);
  assert.match(backend, /r\.FormValue\("class_name"\)/);
  assert.match(backend, /photoImportDefaultAngleName\(r\)/);
  assert.match(backend, /photoImportAngleForSource\(source\.Name, defaultAngle\)/);
  assert.match(backend, /CreateStudentWithFaceVersion\([\s\S]*?angle/);
  assert.match(backend, /AddFaceSampleWithVersion\([\s\S]*?angle/);
  assert.match(backend, /batchStudents := make\(map\[string\]store\.Student\)/);
  assert.match(backend, /photoImportSameStudentDifferentAngle\(item, previous\)/);
  assert.match(backend, /photoImportClassForSource\(source\.Name, defaultClassName, classNames\)/);
  const classes = fs.readFileSync(path.join(assets, 'js', 'classes.js'), 'utf8');
  assert.match(classes, /renderPhotoImportClassOptions\(\)/);
  assert.match(script, /fd\.append\('files', file, file\.name\)/);
  assert.match(script, /fd\.append\('file', zipFiles\[0\], zipFiles\[0\]\.name\)/);
  assert.match(backend, /r\.MultipartForm\.File\["files"\]/);
  assert.match(backend, /r\.MultipartForm\.File\["file"\]/);
  assert.match(backend, /photoImportIdentity\(displayName\)/);
});


test('non-importable and skipped photos are removed from the current batch list', () => {
  const script = read('js/photo_import.js').replace(/\r\n/g, '\n');
  const backend = fs.readFileSync(path.join(root, 'internal', 'web', 'photo_import.go'), 'utf8');
  const extractFunction = name => {
    const start = script.indexOf('function ' + name + '(');
    assert.ok(start >= 0, 'missing helper ' + name);
    const end = script.indexOf('\n}\n', start);
    assert.ok(end > start, 'unterminated helper ' + name);
    return script.slice(start, end + 2);
  };
  const helpers = new Function(
    'canAddStudents',
    'canResampleFaces',
    extractFunction('filterPhotoImportPreviewItems') + '\n' +
    extractFunction('removePhotoImportItemFromList') +
    '\nreturn {filterPhotoImportPreviewItems, removePhotoImportItemFromList};'
  )(true, true);
  const rows = [
    {id: 1, status: 'ready'},
    {id: 2, status: 'existing'},
    {id: 3, status: 'rejected'},
    {id: 4, status: 'review'},
    {id: 5, status: 'package_duplicate'}
  ];
  assert.deepEqual(helpers.filterPhotoImportPreviewItems(rows, true, true), rows.slice(0, 2));
  assert.deepEqual(helpers.filterPhotoImportPreviewItems(rows, false, true), [rows[1]]);
  assert.deepEqual(helpers.filterPhotoImportPreviewItems(rows, false, false), []);
  assert.deepEqual(helpers.removePhotoImportItemFromList(rows.slice(0, 2), 1), [rows[1]]);
  assert.match(script, /photoImportItems = filterPhotoImportPreviewItems\(\s*allItems,\s*hasPermission\('can_add_students'\),\s*hasPermission\('can_resample_faces'\)\s*\)/);
  assert.match(script, /updatePhotoImportRowState\(Number\(select\.dataset\.importAction\), true\)/);
  assert.match(script, /if \(userInitiated && action === 'skip'\) \{\s*if \(removePhotoImportItem\(id\)\)/);
  assert.match(script, /if \(row\.status === 'skipped'\) \{\s*removePhotoImportItem\(row\.id\)/);
  assert.match(script, /return !item\.committed/);
  assert.match(script, /已排除\/跳过/);

  const skipIndex = backend.indexOf('if requested.Action == "skip" || requested.Action == ""');
  const insertIndex = backend.indexOf('case "new":', skipIndex);
  assert.ok(skipIndex >= 0 && insertIndex > skipIndex,
    'the backend must handle skip before any student/sample enrollment action');
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
