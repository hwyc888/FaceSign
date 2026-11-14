const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const jsDir = path.join(__dirname, '..', 'internal', 'web', 'assets', 'js');

function checkinContext(names = ['一班', '二班'], request = async url => ({
  class: {name: new URL(url, 'http://localhost').searchParams.get('class_name')}
})) {
  const calls = [];
  const renders = [];
  const elements = {
    '#checkinSeatClass': {
      value: names[0] || '',
      addEventListener(event, handler) { this[event] = handler; },
      set innerHTML(value) { this.html = value; this.value = ''; }
    },
    '#checkinSeatBoard': {innerHTML: ''},
    '#checkinSeatSummary': {innerHTML: ''},
    '#checkinSeatUnassigned': {classList: {add() {}, remove() {}}}
  };
  const context = vm.createContext({
    $: selector => elements[selector],
    $$: () => [],
    classesCache: names.map(name => ({name})),
    today: () => '2026-11-14',
    esc: value => String(value),
    api: url => { calls.push(url); return request(url); },
    setInterval: () => 0
  });
  vm.runInContext(fs.readFileSync(path.join(jsDir, 'seating.js'), 'utf8'), context);
  context.renderCheckinSeatBoard = board => {
    vm.runInContext('lastRenderedCheckinSeatClass = ' + JSON.stringify(board.class.name), context);
    renders.push(board.class.name);
  };
  return {context, elements, calls, renders, select: elements['#checkinSeatClass']};
}

const face = (id, className, extras = {}) => ({
  matched: true, student: {id, class_name: className}, ...extras
});

test('matched student selects and loads the corresponding class without a manual choice', async () => {
  const {context, select, calls, renders} = checkinContext();
  assert.equal(await context.selectCheckinSeatClassFromRecognition({faces: [face(2, '二班')]}), true);
  assert.equal(select.value, '二班');
  assert.equal(calls.length, 1);
  assert.deepEqual(renders, ['二班']);
  assert.equal(await context.selectCheckinSeatClassFromRecognition({faces: [face(2, '二班')]}), false);
  assert.equal(calls.length, 1);
});

test('switches for a newly arrived student but stays stable when two classes remain visible', async () => {
  const {context, select, calls, renders} = checkinContext();
  assert.equal(await context.selectCheckinSeatClassFromRecognition({faces: [face(1, '一班')]}), true);
  assert.deepEqual(renders, ['一班']);
  assert.equal(await context.selectCheckinSeatClassFromRecognition({faces: [face(1, '一班'), face(2, '二班')]}), true);
  assert.equal(select.value, '二班');
  assert.equal(await context.selectCheckinSeatClassFromRecognition({faces: [face(1, '一班'), face(2, '二班')]}), false);
  assert.equal(select.value, '二班');
  assert.equal(calls.length, 2);
});

test('prioritizes a newly signed-in student when multiple classes are recognized', async () => {
  const {context, select} = checkinContext();
  await context.selectCheckinSeatClassFromRecognition({faces: [face(1, '一班'), face(2, '二班', {recognized: true, first_checkin_today: true})]});
  assert.equal(select.value, '二班');
});

test('unregistered or rejected photo cannot switch classes', async () => {
  const {context, select, calls} = checkinContext();
  await context.selectCheckinSeatClassFromRecognition({faces: [
    {matched: false, student: {id: 2, class_name: '二班'}},
    face(3, '二班', {status: '疑似照片/屏幕'})
  ]});
  assert.equal(select.value, '一班');
  assert.equal(calls.length, 0);
});

test('recognition refreshes class options when the class list has not loaded yet', async () => {
  const {context, select, calls, renders} = checkinContext([]);
  context.api = async url => {
    calls.push(url);
    if (url === '/api/classes') return [{name: '一班'}, {name: '二班'}];
    return {class: {name: '二班'}};
  };
  assert.equal(await context.selectCheckinSeatClassFromRecognition({faces: [face(2, '二班')]}), true);
  assert.equal(select.value, '二班');
  assert.equal(calls[0], '/api/classes');
  assert.deepEqual(renders, ['二班']);
  select.value = '一班';
  select.change();
  context.renderCheckinSeatClassOptions();
  assert.equal(select.value, '一班');
});

test('late class response cannot replace a newer class seat board', async () => {
  const pending = [];
  const {context, select, renders} = checkinContext(['一班', '二班'], () =>
    new Promise((resolve, reject) => pending.push({resolve, reject})));
  const older = context.loadCheckinSeatBoard();
  select.value = '二班';
  const newer = context.loadCheckinSeatBoard();
  pending[1].resolve({class: {name: '二班'}});
  await newer;
  pending[0].resolve({class: {name: '一班'}});
  await older;
  assert.deepEqual(renders, ['二班']);
});

test('recognition flow calls automatic class selection before attendance refresh', async () => {
  let selected = 0;
  let refreshed = 0;
  let attendance = 0;
  const response = {faces: [face(2, '二班', {recognized: true})], verified_count: 1};
  const context = vm.createContext({
    $: () => ({addEventListener() {}}),
    performance: {now: () => 42},
    activeCamera: {id: 1, kind: 'network'},
    recognitionSessionID: 'test-session',
    recognizing: false,
    api: async () => response,
    renderRecognition: () => {},
    toggleCamera: () => {},
    loadToday: () => { attendance++; },
    selectCheckinSeatClassFromRecognition: () => { selected++; return true; },
    loadCheckinSeatBoard: () => { refreshed++; }
  });
  vm.runInContext(fs.readFileSync(path.join(jsDir, 'recognition.js'), 'utf8'), context);
  context.renderRecognition = () => {};
  await context.recognizeFrame({silent: true});
  assert.equal(selected, 1);
  assert.equal(attendance, 1);
  assert.equal(refreshed, 0, 'class switch already triggered the only seat-board refresh');
});

test('recognized student without a class shows a specific data warning instead of asking to create a class', async () => {
  const {context, select, elements, calls} = checkinContext();
  const handled = await context.selectCheckinSeatClassFromRecognition({
    faces: [{recognized: true, student: {id: 8, name: '未分班学生', class_name: ''}}]
  });
  assert.equal(handled, true);
  assert.equal(select.value, '');
  assert.match(elements['#checkinSeatBoard'].innerHTML, /未分班学生/);
  assert.match(elements['#checkinSeatBoard'].innerHTML, /学生管理/);
  assert.equal(calls.length, 0);
});

test('already-selected class still loads its seat board when no board has rendered', async () => {
  const {context, calls, renders} = checkinContext();
  assert.equal(await context.selectCheckinSeatClassFromRecognition({faces: [face(1, '一班')]}), true);
  assert.equal(calls.length, 1);
  assert.deepEqual(renders, ['一班']);
});

test('recognized person track selects its class when no face result is visible in the current frame', async () => {
  const {context, select, renders} = checkinContext();
  await context.selectCheckinSeatClassFromRecognition({
    faces: [],
    persons: [{track_id: 'person-1', student: {id: 7, name: '持续跟踪学生', class_name: '二班'}}]
  });
  assert.equal(select.value, '二班');
  assert.deepEqual(renders, ['二班']);
});

test('manual class selection loads its seat board without any recognition result', async () => {
  let resolveBoard;
  const {select, calls, renders, elements} = checkinContext(
    ['一班', '二班'],
    () => new Promise(resolve => { resolveBoard = resolve; })
  );

  select.value = '二班';
  const loading = select.change();
  assert.equal(calls.length, 1);
  assert.equal(new URL(calls[0], 'http://localhost').searchParams.get('class_name'), '二班');
  assert.match(elements['#checkinSeatSummary'].innerHTML, /正在加载 二班 班签到状态/);
  assert.match(elements['#checkinSeatBoard'].innerHTML, /正在加载 二班 班座位图/);

  resolveBoard({class: {name: '二班'}});
  await loading;
  assert.deepEqual(renders, ['二班']);
});

async function openCheckinPage(classesLoaded) {
  const calls = [];
  const classList = {add() {}, remove() {}, toggle() {}};
  const navButton = {
    dataset: {page: 'checkin'},
    classList,
    addEventListener() {}
  };
  const page = {classList};
  const selectors = {
    '#page-checkin': page,
    '#pageTitle': {textContent: ''},
    '#pageHint': {textContent: ''}
  };
  const context = vm.createContext({
    $: selector => selectors[selector],
    $$: selector => selector === '.nav' ? [navButton] : selector === '.page' ? [page] : [],
    titles: {checkin: ['人脸签到', '摄像头可同时识别多人并完成签到']},
    loadClasses: async () => { calls.push('classes'); return classesLoaded; },
    loadCheckinSeatBoard: async () => { calls.push('seat-board'); },
    enterCheckinPageAutoStart: async () => { calls.push('camera'); }
  });
  vm.runInContext(fs.readFileSync(path.join(jsDir, 'navigation.js'), 'utf8'), context);
  await context.showPage('checkin');
  return calls;
}

test('opening check-in refreshes class list and renders seat board before camera auto-start', async () => {
  assert.deepEqual(await openCheckinPage(true), ['classes', 'seat-board', 'camera']);
});

test('class API failure does not overwrite the visible error with an empty seat board', async () => {
  assert.deepEqual(await openCheckinPage(false), ['classes', 'camera']);
});

function loadClassList(request) {
  const elements = {
    '#checkinSeatClass': {innerHTML: ''},
    '#checkinSeatSummary': {innerHTML: ''},
    '#checkinSeatBoard': {innerHTML: ''}
  };
  const toasts = [];
  const context = vm.createContext({
    $: selector => elements[selector] || null,
    $$: () => [],
    classesCache: [],
    api: request,
    esc: value => String(value),
    toast: message => toasts.push(message),
    renderCheckinSeatClassOptions() {}
  });
  const source = fs.readFileSync(path.join(jsDir, 'classes.js'), 'utf8');
  const start = source.indexOf('function renderClassOptions');
  const end = source.indexOf('async function moveClass');
  vm.runInContext(source.slice(start, end), context);
  return {context, elements, toasts};
}

test('class list API failure is visible in the check-in seat board', async () => {
  const {context, elements, toasts} = loadClassList(async () => {
    throw new Error('班级接口连接失败');
  });
  assert.equal(await context.loadClasses(), false);
  assert.match(elements['#checkinSeatClass'].innerHTML, /班级列表加载失败/);
  assert.match(elements['#checkinSeatSummary'].innerHTML, /班级列表加载失败/);
  assert.match(elements['#checkinSeatBoard'].innerHTML, /班级接口连接失败/);
  assert.deepEqual(toasts, ['班级接口连接失败']);
});

test('an empty class list is a successful load and can show the no-class state', async () => {
  const {context} = loadClassList(async () => []);
  assert.equal(await context.loadClasses(), true);
  assert.deepEqual(context.classesCache, []);
});

