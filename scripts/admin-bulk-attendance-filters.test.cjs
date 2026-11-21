const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const root = path.join(__dirname, '..');
const jsDir = path.join(root, 'internal', 'web', 'assets', 'js');
const read = relative => fs.readFileSync(path.join(root, relative), 'utf8').replace(/\r\n/g, '\n');

function extractFunction(source, name) {
  const start = source.indexOf('function ' + name + '(');
  assert.ok(start >= 0, 'missing helper ' + name);
  const nextFunction = source.indexOf('\nfunction ', start + 1);
  const nextAsyncFunction = source.indexOf('\nasync function ', start + 1);
  const candidates = [nextFunction, nextAsyncFunction].filter(index => index >= 0);
  const end = candidates.length ? Math.min(...candidates) : source.length;
  const block = source.slice(start, end);
  const brace = block.lastIndexOf('}');
  assert.ok(brace > 0, 'unterminated helper ' + name);
  return block.slice(0, brace + 1);
}

test('bulk administrator role selection excludes the primary admin and separates grant from revoke', () => {
  const source = fs.readFileSync(path.join(jsDir, 'users.js'), 'utf8').replace(/\r\n/g, '\n');
  const helper = extractFunction(source, 'filterUsersForAdminRoleAction');
  const filterUsers = new Function(helper + '\nreturn filterUsersForAdminRoleAction;')();
  const users = [
    {id: 1, username: 'primary', is_admin: true, is_primary_admin: true},
    {id: 2, username: 'teacher', is_admin: false, is_primary_admin: false},
    {id: 3, username: 'admin-a', is_admin: true, is_primary_admin: false},
    {id: 4, username: 'teacher-b', is_admin: false, is_primary_admin: false}
  ];
  const selected = new Set([1, 2, 3, 4]);
  assert.deepEqual(filterUsers(users, selected, true).map(user => user.id), [2, 4]);
  assert.deepEqual(filterUsers(users, selected, false).map(user => user.id), [3]);
  assert.match(source, /currentUser\.is_primary_admin/);
  assert.match(source, /method: makeAdmin \? 'POST' : 'DELETE'/);
  assert.match(source, /data-user-select/);
  assert.match(source, /bulkGrantAdmins/);
  assert.match(source, /bulkRevokeAdmins/);
});

test('attendance filters combine class, student name and student number', () => {
  const source = fs.readFileSync(path.join(jsDir, 'attendance.js'), 'utf8').replace(/\r\n/g, '\n');
  const helper = extractFunction(source, 'filterAttendanceRecords');
  const filterRecords = new Function(helper + '\nreturn filterAttendanceRecords;')();
  const records = [
    {student_no: '2026001', name: '张三', class_name: '1班'},
    {student_no: '2026002', name: '李四', class_name: '1班'},
    {student_no: '2026003', name: '王五', class_name: '2班'}
  ];
  assert.deepEqual(filterRecords(records, '1班', '').map(item => item.student_no), ['2026001', '2026002']);
  assert.deepEqual(filterRecords(records, '', '李四').map(item => item.student_no), ['2026002']);
  assert.deepEqual(filterRecords(records, '', '2026003').map(item => item.name), ['王五']);
  assert.deepEqual(filterRecords(records, '2班', '李四'), []);
});

test('attendance and account filter controls are present and responsive styles prevent overflow', () => {
  const html = read('internal/web/assets/index.html');
  const css = read('internal/web/assets/style.css');
  assert.match(html, /id="attendanceClassFilter"/);
  assert.match(html, /id="attendanceStudentSearch"/);
  assert.match(html, /id="userBulkToolbar"/);
  assert.match(html, /id="userSelectAll"/);
  assert.match(html, /id="bulkGrantAdmins"/);
  assert.match(html, /id="bulkRevokeAdmins"/);
  assert.match(html, /<th>选择<\/th>/);
  assert.match(css, /@media \(max-width: 1100px\)/);
  assert.match(css, /\.user-table tbody tr:not\(\.user-table-empty\)/);
  assert.match(css, /\.attendance-filters/);
});
