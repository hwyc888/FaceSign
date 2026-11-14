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
  context.renderCheckinSeatBoard = board => renders.push(board.class.name);
  return {context, elements, calls, renders, select: elements['#checkinSeatClass']};
}

const face = (id, className, extras = {}) => ({
  matched: true, student: {id, class_name: className}, ...extras
});

test('matched student selects and loads the corresponding class without a manual choice', async () => {
  const {context, select, calls, renders} = checkinContext();
  assert.equal(context.selectCheckinSeatClassFromRecognition({faces: [face(2, '二班')]}), true);
  assert.equal(select.value, '二班');
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(calls.length, 1);
  assert.deepEqual(renders, ['二班']);
  assert.equal(context.selectCheckinSeatClassFromRecognition({faces: [face(2, '二班')]}), false);
  assert.equal(calls.length, 1);
});

test('switches for a newly arrived student but stays stable when two classes remain visible', async () => {
  const {context, select, calls} = checkinContext();
  assert.equal(context.selectCheckinSeatClassFromRecognition({faces: [face(1, '一班')]}), false);
  assert.equal(context.selectCheckinSeatClassFromRecognition({faces: [face(1, '一班'), face(2, '二班')]}), true);
  assert.equal(select.value, '二班');
  assert.equal(context.selectCheckinSeatClassFromRecognition({faces: [face(1, '一班'), face(2, '二班')]}), false);
  assert.equal(select.value, '二班');
  assert.equal(calls.length, 1);
});

test('prioritizes a newly signed-in student when multiple classes are recognized', () => {
  const {context, select} = checkinContext();
  context.selectCheckinSeatClassFromRecognition({faces: [face(1, '一班'), face(2, '二班', {recognized: true, first_checkin_today: true})]});
  assert.equal(select.value, '二班');
});

test('unregistered or rejected photo cannot switch classes', () => {
  const {context, select, calls} = checkinContext();
  context.selectCheckinSeatClassFromRecognition({faces: [
    {matched: false, student: {id: 2, class_name: '二班'}},
    face(3, '二班', {status: '疑似照片/屏幕'})
  ]});
  assert.equal(select.value, '一班');
  assert.equal(calls.length, 0);
});

test('recognition before class options finish loading is applied on class refresh', () => {
  const {context, select} = checkinContext([]);
  assert.equal(context.selectCheckinSeatClassFromRecognition({faces: [face(2, '二班')]}), false);
  context.classesCache.push({name: '一班'}, {name: '二班'});
  context.renderCheckinSeatClassOptions();
  assert.equal(select.value, '二班');
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
