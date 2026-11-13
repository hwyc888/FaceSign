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
  const autoScan = {checked: false, addEventListener() {}};
  const checkinPage = {classList: {contains: () => true}};
  const context = vm.createContext({
    $: selector => {
      if (selector === '#faceOverlay') return overlay;
      if (selector === '#autoScan') return autoScan;
      if (selector === '#page-checkin') return checkinPage;
      return {addEventListener() {}};
    },
    setTimeout(callback, delay) { timers.set(++nextTimer, {callback, delay}); return nextTimer; },
    clearTimeout(timer) { timers.delete(timer); },
    performance: {now: () => 0},
    recognizing: false, activeCamera: {id: 1, kind: 'network'},
    cameraOpen: true, manualRecognitionBurst: false,
    recognitionSessionID: 'camera-one', autoTimer: null,
    toast() {},
    toggleCamera() {},
    api: async () => ({skipped: true, busy: true})
  });
  const script = fs.readFileSync(path.join(__dirname, '../internal/web/assets/js/recognition.js'), 'utf8');
  vm.runInContext(script, context);
  return {context, rectangles, labels, timers, autoScan, clearCount: () => clearCount};
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

function createMotionHarness() {
  const harness = createHarness();
  harness.context.document = {createElement: () => ({})};
  vm.runInContext(fs.readFileSync(path.join(__dirname, '../internal/web/assets/js/tracking.js'), 'utf8'), harness.context);
  return harness;
}

function createTextureFrame(offsetX = 0, time = 0, scale = 1) {
  const width = 160;
  const height = 120;
  const gray = new Uint8Array(width * height);
  for (let vertical = 0; vertical < height; vertical++) {
    for (let horizontal = 0; horizontal < width; horizontal++) {
      const sourceX = (horizontal - offsetX - width / 2) / scale + width / 2;
      const sourceY = (vertical - height / 2) / scale + height / 2;
      const texture =
        128 +
        42 * Math.sin(sourceX * 0.11) +
        37 * Math.cos(sourceY * 0.13) +
        24 * Math.sin((sourceX + sourceY) * 0.07) +
        18 * Math.cos((sourceX - sourceY) * 0.17);
      gray[vertical * width + horizontal] = Math.max(0, Math.min(255, Math.round(texture)));
    }
  }
  return {width, height, gray, time};
}

test('local texture tracking moves boxes from pixels without waiting for identity', () => {
  const harness = createMotionHarness();
  const previous = createTextureFrame();
  const current = createTextureFrame(6, 33);
  const box = {x: 0.3, y: 0.3, width: 0.2, height: 0.3};
  const moved = harness.context.trackTextureBox(previous, current, box);
  assert.ok(moved);
  assert.ok(Math.abs(moved.x - box.x - 6 / current.width) < 0.0001);
  assert.ok(Math.abs(moved.y - box.y) < 0.0001);
  assert.equal(harness.context.trackTextureBox(previous, {...current, time: 1000}, box), null);
});

test('ambiguous overlapping people lose labels rather than exchanging identities', () => {
  const harness = createMotionHarness();
  const previous = createTextureFrame();
  const current = createTextureFrame(4, 33);
  const box = {x: 0.3, y: 0.3, width: 0.2, height: 0.3};
  const result = harness.context.advanceTrackingBoxes(previous, current, [
    {confirmedAt: 0, face: {student: {id: 1}}, box},
    {confirmedAt: 0, face: {student: {id: 2}}, box: {...box, x: 0.32}}
  ]);
  assert.equal(result.length, 0);
});

test('recognition captures a motion reference and forwards it with the exact response', async () => {
  const harness = createHarness();
  const reference = {sequence: 42};
  const response = {faces: [], persons: []};
  harness.context.beginTrackingReference = () => reference;
  harness.context.api = async () => response;
  let receivedReference;
  harness.context.renderRecognition = (result, trackingReference) => {
    assert.equal(result, response);
    receivedReference = trackingReference;
  };
  await harness.context.recognizeFrame({silent: true});
  assert.equal(receivedReference, reference);
  let stopped = false;
  harness.context.stopRealtimeTracking = () => { stopped = true; };
  harness.context.stopAutoRecognition();
  assert.equal(stopped, true);
});


test('server camera body boxes render before full face recognition returns', async () => {
  const harness = createHarness();
  harness.context.activeCamera = {id: 42, kind: 'local'};
  const early = {
    persons: [{track_id: 'camera-P1', box: {x: 10, y: 20, width: 100, height: 200}}],
    frame_width: 640, frame_height: 480, tracked_person_count: 1
  };
  const final = {faces: [], persons: [], frame_width: 640, frame_height: 480};
  const calls = [];
  const rendered = [];
  harness.context.api = async url => {
    calls.push(url);
    return url.endsWith('/track') ? early : final;
  };
  harness.context.renderRecognition = result => rendered.push(result);
  const result = await harness.context.recognizeFrame({silent: true});
  assert.equal(result, final);
  assert.deepEqual(calls, ['/api/cameras/42/track', '/api/cameras/42/recognize']);
  assert.equal(rendered[0], early);
  assert.equal(rendered[1], final);
});

test('manual browser recognition enables local motion tracking', () => {
  const harness = createMotionHarness();
  harness.context.manualRecognitionBurst = true;
  assert.equal(harness.context.realtimeTrackingEnabled(), true);
});

test('browser tracking keeps body and face boxes independently visible', () => {
  const harness = createMotionHarness();
  const reference = createTextureFrame(0, 0);
  reference.sequence = 42;
  const current = createTextureFrame(6, 33);
  current.sequence = 43;
  harness.autoScan.checked = true;
  harness.context.testReference = reference;
  harness.context.testCurrent = current;
  vm.runInContext(
    'activeCamera = {id: 1, kind: "browser"}; recognitionSessionID = "camera-one"; realtimeTrackingSession = "camera-one"; realtimeTrackingFrames = [testReference, testCurrent];',
    harness.context
  );
  const result = {
    frame_width: 160, frame_height: 120,
    faces: [{recognized: true, student: {name: 'Amy'}, box: {x: 48, y: 36, width: 32, height: 36}}],
    persons: [{track_id: 'camera-one-P1', status: '跟踪中', box: {x: 32, y: 20, width: 96, height: 92}}]
  };
  assert.equal(harness.context.applyTrackedRecognition(result, reference), true);
  assert.equal(harness.rectangles.length, 2);
  assert.ok(harness.labels.some(label => label.includes('P1')));
  assert.ok(harness.labels.some(label => label.includes('Amy')));
});

test('local tracking adjusts box size as a person approaches', () => {
  const harness = createMotionHarness();
  const previous = createTextureFrame(0, 0, 1);
  const current = createTextureFrame(0, 33, 1.12);
  const box = {x: 0.25, y: 0.20, width: 0.35, height: 0.50};
  const moved = harness.context.trackTextureBox(previous, current, box);
  assert.ok(moved);
  assert.ok(moved.width > box.width * 1.02, `width did not expand: ${moved.width}`);
  assert.ok(moved.height > box.height * 1.02, `height did not expand: ${moved.height}`);
});
