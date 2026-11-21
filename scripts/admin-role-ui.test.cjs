const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const jsDir = path.join(__dirname, '..', 'internal', 'web', 'assets', 'js');
const authSource = fs.readFileSync(path.join(jsDir, 'auth.js'), 'utf8');

function permissionContext(user) {
  const changes = [];
  const makeElement = name => ({
    classList: {
      toggle(className, hidden) {
        changes.push({name, className, hidden: Boolean(hidden)});
      }
    },
    textContent: ''
  });
  const adminOnly = [makeElement('admin-only-page'), makeElement('admin-only-threshold')];
  const permissionOnly = [makeElement('settings-permission')];
  const ordinary = new Map([
    ['#enrollmentWorkbench', makeElement('enrollment-workbench')],
    ['#photoImportCard', makeElement('photo-import')],
    ['#sampleControls', makeElement('sample-controls')],
    ['#startSupplement', makeElement('start-supplement')],
    ['#duplicateUpdateProfile', makeElement('duplicate-update')],
    ['#currentUserLabel', makeElement('current-user-label')]
  ]);
  const context = vm.createContext({
    currentUser: user,
    document: {body: {classList: {toggle(name, value) { changes.push({name, hidden: Boolean(value)}); }}}},
    $$: selector => selector === '[data-admin-only]' ? adminOnly : selector === '[data-permission]' ? permissionOnly : [],
    $: selector => ordinary.get(selector) || null
  });
  const hasStart = authSource.indexOf('function hasPermission(name) {');
  const hasEnd = authSource.indexOf('function showAuthPanel', hasStart);
  const applyStart = authSource.indexOf('function applyUserPermissions() {');
  const applyEnd = authSource.indexOf('function enterAuthenticatedApp()', applyStart);
  assert.notEqual(hasStart, -1);
  assert.notEqual(hasEnd, -1);
  assert.notEqual(applyStart, -1);
  assert.notEqual(applyEnd, -1);
  vm.runInContext(authSource.slice(hasStart, hasEnd) + authSource.slice(applyStart, applyEnd), context);
  context.applyUserPermissions();
  return {changes, label: ordinary.get('#currentUserLabel').textContent};
}

test('delegated administrator sees every admin-only page and threshold control', () => {
  const result = permissionContext({
    id: 2,
    username: 'delegated',
    is_admin: true,
    is_primary_admin: false
  });
  const adminControls = result.changes.filter(change =>
    change.name === 'admin-only-page' || change.name === 'admin-only-threshold'
  );
  assert.equal(adminControls.length, 2);
  assert.ok(adminControls.every(change => change.hidden === false));
  assert.equal(result.label, '管理员：delegated');
});

test('primary administrator is clearly identified in the UI', () => {
  const result = permissionContext({
    id: 1,
    username: 'primary',
    is_admin: true,
    is_primary_admin: true
  });
  assert.equal(result.label, '首个管理员：primary');
});

test('ordinary users do not see admin-only pages or threshold controls', () => {
  const result = permissionContext({
    id: 3,
    username: 'teacher',
    is_admin: false,
    is_primary_admin: false,
    can_edit_settings: true
  });
  const adminControls = result.changes.filter(change =>
    change.name === 'admin-only-page' || change.name === 'admin-only-threshold'
  );
  assert.equal(adminControls.length, 2);
  assert.ok(adminControls.every(change => change.hidden === true));
  assert.equal(result.label, '用户：teacher');
});

test('only the primary administrator UI exposes administrator grant and revoke actions', () => {
  const usersSource = fs.readFileSync(path.join(jsDir, 'users.js'), 'utf8');
  assert.match(usersSource, /const canManageAdminRoles = Boolean\(currentUser\.is_primary_admin\)/);
  assert.match(usersSource, /data-user-grant-admin/);
  assert.match(usersSource, /data-user-revoke-admin/);
});
