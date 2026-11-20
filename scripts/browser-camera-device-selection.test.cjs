const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const jsDir = path.join(__dirname, '..', 'internal', 'web', 'assets', 'js');

function extractFunction(source, signature, nextSignature) {
  const start = source.indexOf(signature);
  assert.notEqual(start, -1, `missing function: ${signature}`);
  const end = source.indexOf(nextSignature, start);
  assert.notEqual(end, -1, `missing function boundary: ${nextSignature}`);
  return source.slice(start, end);
}

function formContext(kind, selectedDevice) {
  const values = {
    '#cameraKind': {value: kind},
    '#cameraBrowserFacing': {value: selectedDevice},
    '#cameraName': {value: '浏览器 USB 摄像头'},
    '#cameraProtocol': {value: 'browser'},
    '#cameraStreamURL': {value: ''},
    '#cameraSnapshotURL': {value: ''},
    '#cameraUsername': {value: ''},
    '#cameraAuthMode': {value: 'none'},
    '#cameraAgentID': {value: ''},
    '#cameraWidth': {value: '1920'},
    '#cameraHeight': {value: '1080'},
    '#cameraFPS': {value: '30'},
    '#cameraTimeout': {value: '3000'},
    '#cameraTLSInsecure': {checked: false},
    '#cameraDefault': {checked: false},
    '#cameraClearPassword': {checked: false},
    '#cameraPassword': {value: ''},
    '#cameraAgentSecret': {value: ''}
  };
  const context = vm.createContext({$: selector => values[selector], editingCameraID: 0});
  const source = fs.readFileSync(path.join(jsDir, 'cameras.js'), 'utf8');
  vm.runInContext(extractFunction(source, 'function cameraFormPayload() {', 'function renderBrowserCameraDeviceOptions'), context);
  return context.cameraFormPayload();
}

test('browser camera form sends an enumerated USB device ID unchanged', () => {
  const id = 'opaque-browser-device-id-3277-0029';
  const payload = formContext('browser', id);
  assert.equal(payload.kind, 'browser');
  assert.equal(payload.protocol, 'browser');
  assert.equal(payload.device_id, id);
});

test('automatic browser camera selection remains an empty device ID', () => {
  const payload = formContext('browser', '');
  assert.equal(payload.device_id, '');
});

test('refreshing browser camera options preserves automatic selection and exposes enumerated USB devices', () => {
  const select = {
    _value: '',
    options: [],
    set innerHTML(html) {
      this.html = html;
      this.options = Array.from(html.matchAll(/<option value="([^"]*)">/g), match => match[1]);
      if (!this.options.includes(this._value)) this._value = '';
    },
    get value() { return this._value; },
    set value(value) { this._value = this.options.includes(String(value)) ? String(value) : ''; }
  };
  const context = vm.createContext({
    $: selector => selector === '#cameraBrowserFacing' ? select : null,
    esc: value => String(value),
    browserCameraDevices: []
  });
  const source = fs.readFileSync(path.join(jsDir, 'cameras.js'), 'utf8');
  vm.runInContext(extractFunction(source, 'function renderBrowserCameraDeviceOptions(', 'async function refreshBrowserCameraDevices'), context);

  context.renderBrowserCameraDeviceOptions();
  assert.equal(select.value, '', 'empty selection must remain automatic, not become front-facing');

  context.browserCameraDevices = [{deviceId: 'usb-device-123', label: 'USB2.0 HD UVC WebCam'}];
  context.renderBrowserCameraDeviceOptions('usb-device-123');
  assert.equal(select.value, 'usb-device-123');
  assert.match(select.html, /USB2\.0 HD UVC WebCam/);
});

test('selected browser USB device uses an exact deviceId constraint', () => {
  const source = fs.readFileSync(path.join(jsDir, 'camera.js'), 'utf8');
  const context = vm.createContext({});
  vm.runInContext(extractFunction(source, 'function localVideoConstraints(camera) {', 'function browserCameraOpenErrorIsRetryable'), context);
  const constraints = context.localVideoConstraints({kind: 'browser', device_id: 'usb-device-123', width: 1920, height: 1080, fps: 30});
  assert.equal(constraints.video.deviceId.exact, 'usb-device-123');
  assert.equal('facingMode' in constraints.video, false);
});

test('front and rear preferences use facingMode while automatic mode does not force front-facing', () => {
  const source = fs.readFileSync(path.join(jsDir, 'camera.js'), 'utf8');
  const context = vm.createContext({});
  vm.runInContext(extractFunction(source, 'function localVideoConstraints(camera) {', 'function browserCameraOpenErrorIsRetryable'), context);
  assert.equal(context.localVideoConstraints({kind: 'browser', device_id: 'user'}).video.facingMode.ideal, 'user');
  assert.equal(context.localVideoConstraints({kind: 'browser', device_id: 'environment'}).video.facingMode.ideal, 'environment');
  const automatic = context.localVideoConstraints({kind: 'browser', device_id: ''});
  assert.equal('facingMode' in automatic.video, false);
  assert.equal('deviceId' in automatic.video, false);
});
