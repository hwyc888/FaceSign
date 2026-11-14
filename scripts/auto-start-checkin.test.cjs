const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const jsDir = path.join(__dirname, '..', 'internal', 'web', 'assets', 'js');
const settingsSource = fs.readFileSync(path.join(jsDir, 'settings.js'), 'utf8');
const settingsEnd = settingsSource.indexOf("$('#autoStartCheckin').addEventListener");
assert.notEqual(settingsEnd, -1, 'settings module must expose the auto-start handler');

function settingsContext({
  settingsResponse,
  pageActive = true,
  initialSettings = null,
  cameraOpen = false
}) {
  let currentPageActive = pageActive;
  const checkbox = {checked: false};
  const state = {textContent: ''};
  const page = {
    classList: {
      contains: name => name === 'active' && currentPageActive
    }
  };
  const elements = {
    '#autoStartCheckin': checkbox,
    '#autoStartCheckinState': state,
    '#page-checkin': page
  };
  const apiCalls = [];
  const recognitionCalls = [];
  const toasts = [];
  let cameraStarts = 0;

  const context = vm.createContext({
    $: selector => elements[selector] || null,
    $$: () => [],
    api: async url => {
      apiCalls.push(url);
      return typeof settingsResponse === 'function'
        ? await settingsResponse(url)
        : await settingsResponse;
    },
    setAutoRecognitionEnabled: async enabled => {
      recognitionCalls.push(enabled);
    },
    startCamera: async () => {
      cameraStarts++;
    },
    cameraOpen,
    toast: message => toasts.push(message),
    console: {warn() {}}
  });

  vm.runInContext(settingsSource.slice(0, settingsEnd), context);
  if (initialSettings) {
    vm.runInContext(
      'appSettingsCache = ' + JSON.stringify(initialSettings) + '; appSettingsLoaded = true;',
      context
    );
  }

  return {
    context,
    apiCalls,
    recognitionCalls,
    toasts,
    setPageActive: value => { currentPageActive = value; },
    get cameraStarts() { return cameraStarts; }
  };
}

test('check-in entry refreshes a stale cached setting before deciding to auto-start', async () => {
  const state = settingsContext({
    settingsResponse: {auto_start_checkin: true},
    initialSettings: {auto_start_checkin: false},
    pageActive: true
  });

  await state.context.enterCheckinPageAutoStart();

  assert.deepEqual(state.apiCalls, ['/api/settings']);
  assert.deepEqual(state.recognitionCalls, [true]);
});

test('disabled auto-start does not open the camera or start recognition', async () => {
  const state = settingsContext({
    settingsResponse: {auto_start_checkin: false},
    pageActive: true,
    cameraOpen: true
  });

  await state.context.enterCheckinPageAutoStart();

  assert.deepEqual(state.recognitionCalls, []);
  assert.equal(state.cameraStarts, 0);
});

test('a page transition away from check-in cancels pending automatic startup', async () => {
  let resolveSettings;
  const pendingSettings = new Promise(resolve => {
    resolveSettings = resolve;
  });
  const state = settingsContext({
    settingsResponse: () => pendingSettings,
    pageActive: true
  });

  const startup = state.context.enterCheckinPageAutoStart();
  state.setPageActive(false);
  resolveSettings({auto_start_checkin: true});
  await startup;

  assert.deepEqual(state.recognitionCalls, []);
});

test('simultaneous startup triggers share one settings request and one recognition start', async () => {
  let resolveSettings;
  const pendingSettings = new Promise(resolve => {
    resolveSettings = resolve;
  });
  const state = settingsContext({
    settingsResponse: () => pendingSettings,
    pageActive: true
  });

  const first = state.context.enterCheckinPageAutoStart();
  const second = state.context.enterCheckinPageAutoStart();
  assert.equal(state.apiCalls.length, 1);

  resolveSettings({auto_start_checkin: true});
  await Promise.all([first, second]);

  assert.deepEqual(state.recognitionCalls, [true]);
});

test('automatic startup errors are visible instead of being silently swallowed', async () => {
  const state = settingsContext({
    settingsResponse: async () => {
      throw new Error('settings API unavailable');
    },
    pageActive: true
  });

  await state.context.enterCheckinPageAutoStart();

  assert.equal(state.recognitionCalls.length, 0);
  assert.match(state.toasts[0] || '', /自动启动签到失败/);
});
