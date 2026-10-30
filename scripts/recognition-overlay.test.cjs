const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');
const {test} = require('node:test');

function createHarness() {
  const rectangles = [];
  const labels = [];
  const timers = new Map();
  let nextTimer = 0;
  let clearCount = 0;
  const drawingContext = {
    clearRect() { clearCount++; },
    strokeRect(...coordinates) { rectangles.push(coordinates); },
    fillText(label) { labels.push(label); },
    save() {}, restore() {}, setLineDash() {}
  };
  const overlay = {getContext: () => drawingContext};
  const context = vm.createContext({
    $: selector => selector === '#faceOverlay' ? overlay : {addEventListener() {}},
    setTimeout(callback, delay) { timers.set(++nextTimer, {callback, delay}); return nextTimer; },
    clearTimeout(timer) { timers.delete(timer); },
    performance: {now: () => 0},
    recognizing: false, activeCamera: {id: 1, kind: 'network'},
    recognitionSessionID: 'camera-one', autoTimer: null,
    toast() {},
    toggleCamera() {},
    api: async () => ({skipped: true, busy: true})
  });
  const script = fs.readFileSync(path.join(__dirname, '../internal/web/assets/js/recognition.js'), 'utf8');
  vm.runInContext(script, context);
  return {context, rectangles, labels, timers, clearCount: () => clearCount};
}

test('multiple bodies keep separate boxes without trusting cached body names', () => {
  const harness = createHarness();
  harness.context.drawFaceOverlay([], [
    {track_id: 'camera-P1', face_visible: true, student: {name: 'Old name'}, box: {x: 10, y: 20, width: 100, height: 200}},
    {track_id: 'camera-P2', face_visible: false, box: {x: 220, y: 30, width: 100, height: 200}}
  ], 1280, 720);
  assert.equal(harness.rectangles.length, 2);
  assert.notDeepEqual(harness.rectangles[0], harness.rectangles[1]);
  assert.ok(harness.labels[0].includes('P1'));
  assert.ok(harness.labels[1].includes('P2'));
  assert.ok(harness.labels.every(label => !label.includes('Old name')));
});

test('pending and busy recognition requests do not erase existing boxes', async () => {
  const harness = createHarness();
  harness.context.drawFaceOverlay([], [{track_id: 'P1', box: {x: 10, y: 20, width: 100, height: 200}}]);
  const clearCount = harness.clearCount();
  await harness.context.recognizeFrame({silent: true});
  assert.equal(harness.clearCount(), clearCount);
  assert.equal(harness.timers.size, 1);
});

test('new frames replace old expiry and empty scenes clear immediately', () => {
  const harness = createHarness();
  const people = [{track_id: 'P1', box: {x: 10, y: 20, width: 100, height: 200}}];
  harness.context.drawFaceOverlay([], people);
  harness.context.drawFaceOverlay([], people);
  assert.equal(harness.timers.size, 1);
  const expiry = [...harness.timers.values()][0];
  assert.ok(expiry.delay >= 3000);
  const clearCount = harness.clearCount();
  expiry.callback();
  assert.equal(harness.clearCount(), clearCount + 1);
  harness.context.drawFaceOverlay([], []);
  assert.equal(harness.timers.size, 0);
});

test('disabling automatic recognition immediately clears boxes and expiry', async () => {
  const harness = createHarness();
  harness.context.drawFaceOverlay([], [{track_id: 'P1', box: {x: 10, y: 20, width: 100, height: 200}}]);
  const clearCount = harness.clearCount();
  await harness.context.setAutoRecognitionEnabled(false);
  assert.equal(harness.clearCount(), clearCount + 1);
  assert.equal(harness.timers.size, 0);
});

test('a late response cannot redraw boxes after automatic recognition stops', async () => {
  const harness = createHarness();
  let resolveResponse;
  harness.context.api = () => new Promise(resolve => { resolveResponse = resolve; });
  let renderCount = 0;
  harness.context.renderRecognition = () => { renderCount++; };
  const pendingRequest = harness.context.recognizeFrame({silent: true});
  harness.context.stopAutoRecognition();
  resolveResponse({faces: [{recognized: true}], persons: []});
  assert.equal(await pendingRequest, null);
  assert.equal(renderCount, 0);
  assert.equal(harness.timers.size, 0);
});

function loadMotionTracker() {
  const context = vm.createContext({document: {createElement: () => ({})}, clearTimeout() {}});
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../internal/web/assets/js/tracking.js'), 'utf8'), context);
  return context;
}

function createTextureFrame(offsetX = 0, offsetY = 0) {
  const width = 96;
  const height = 96;
  const gray = new Uint8Array(width * height);
  for (let vertical = 0; vertical < height; vertical++) {
    for (let horizontal = 0; horizontal < width; horizontal++) {
      const sourceX = horizontal - offsetX;
      const sourceY = vertical - offsetY;
      gray[vertical * width + horizontal] = ((sourceX * 73856093) ^ (sourceY * 19349663)) & 255;
    }
  }
  return {width, height, gray, time: 50};
}

test('local texture tracking follows actual image translation without waiting for identity', () => {
  const tracker = loadMotionTracker();
  const box = {x: 0.3, y: 0.3, width: 0.25, height: 0.25};
  const moved = tracker.trackTextureBox(createTextureFrame(), createTextureFrame(6, -4), box);
  assert.ok(moved);
  assert.ok(Math.abs(moved.x - box.x - 6 / 96) < 0.0001);
  assert.ok(Math.abs(moved.y - box.y + 4 / 96) < 0.0001);
});

test('textureless or expired evidence cannot retain a named tracking box', () => {
  const tracker = loadMotionTracker();
  const blank = {width: 96, height: 96, gray: new Uint8Array(96 * 96), time: 4000};
  const box = {x: 0.3, y: 0.3, width: 0.25, height: 0.25};
  assert.equal(tracker.trackTextureBox(blank, blank, box), null);
  assert.equal(tracker.advanceTrackingBoxes(blank, blank, [{box, confirmedAt: 0}]).length, 0);
});

test('separate tracks move independently and overlapping tracks are discarded', () => {
  const tracker = loadMotionTracker();
  const previous = createTextureFrame();
  const current = createTextureFrame(4, 2);
  const first = {box: {x: 0.2, y: 0.2, width: 0.15, height: 0.15}, confirmedAt: 0, face: {student: {id: 1}}};
  const second = {box: {x: 0.6, y: 0.6, width: 0.15, height: 0.15}, confirmedAt: 0, face: {student: {id: 2}}};
  const moved = tracker.advanceTrackingBoxes(previous, current, [first, second]);
  assert.equal(moved.length, 2);
  assert.equal(moved[0].face.student.id, 1);
  assert.equal(moved[1].face.student.id, 2);
  assert.ok(Math.abs(moved[1].box.x - second.box.x - 4 / 96) < 0.0001);
  assert.equal(tracker.advanceTrackingBoxes(previous, current, [first, {...second, box: first.box}]).length, 0);
});
