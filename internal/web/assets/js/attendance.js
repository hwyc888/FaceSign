function today() {
  const d = new Date();
  const p = n => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

async function loadAttendance() {
  const day = $('#attendanceDay').value || today();
  $('#attendanceDay').value = day;
  try {
    const items = await api(`/api/attendance?day=${encodeURIComponent(day)}`);
    $('#attendanceBody').innerHTML = items.map(a => {
      const count = Number(a.snapshot_count || 0);
      const snapshotAction = count > 0
        ? (typeof currentUser !== 'undefined' && currentUser && currentUser.is_admin
          ? '<button type="button" class="small" data-view-attendance-snapshots="' + Number(a.id) + '" data-student-name="' + esc(a.name) + '" data-student-no="' + esc(a.student_no) + '" data-snapshot-count="' + count + '">查看 ' + count + ' 张</button>'
          : '<span class="muted">管理员可查看</span>')
        : '<span class="muted">暂无照片</span>';
      return `
        <tr>
          <td>${esc(a.checked_at)}</td>
          <td>${esc(a.last_seen_at || a.checked_at)}</td>
          <td>${Number(a.recognition_count || 1)} 次</td>
          <td>${esc(a.student_no)}</td>
          <td>${esc(a.name)}</td>
          <td>${esc(a.class_name || '-')}</td>
          <td>${(a.similarity * 100).toFixed(1)}%</td>
          <td>${snapshotAction}</td>
        </tr>
      `;
    }).join('') || '<tr><td colspan="8">暂无记录</td></tr>';
    $('#todaySummary').textContent = day === today() ? `今日已签到 ${items.length} 人` : '';
  } catch (e) {
    toast(e.message);
  }
}

function loadToday() {
  const old = $('#attendanceDay').value;
  $('#attendanceDay').value = today();
  loadAttendance().finally(() => {
    $('#attendanceDay').value = old || today();
  });
}

$('#refreshAttendance').addEventListener('click', loadAttendance);

const attendanceBody = $('#attendanceBody');
attendanceBody.addEventListener('click', async event => {
  const button = event.target.closest('[data-view-attendance-snapshots]');
  if (!button) return;
  const attendanceID = Number(button.dataset.viewAttendanceSnapshots);
  if (!Number.isSafeInteger(attendanceID) || attendanceID <= 0) return;
  await openAttendanceSnapshotTimeline(attendanceID, button.dataset.studentName, button.dataset.studentNo);
});

async function openAttendanceSnapshotTimeline(attendanceID, studentName, studentNo) {
  const modal = $('#attendanceSnapshotModal');
  const timeline = $('#attendanceSnapshotTimeline');
  const error = $('#attendanceSnapshotError');
  timeline.replaceChildren();
  error.classList.add('hidden');
  $('#attendanceSnapshotTitle').textContent = '全天识别照片';
  $('#attendanceSnapshotSubtitle').textContent = (studentName || '') + ' · ' + (studentNo || '') + ' · 正在读取照片…';
  modal.classList.remove('hidden');
  try {
    const items = await api('/api/attendance/' + attendanceID + '/snapshots');
    $('#attendanceSnapshotSubtitle').textContent = (studentName || '') + ' · ' + (studentNo || '') + ' · 共 ' + items.length + ' 张，按时间倒序排列';
    if (!items.length) {
      error.classList.remove('hidden');
      return;
    }
    timeline.innerHTML = items.map(item => `
      <article class="attendance-snapshot-item">
        <div class="attendance-snapshot-item-head">
          <strong>${esc(item.recognized_at)}</strong>
          <span>识别成功</span>
        </div>
        <img src="/api/attendance/snapshots/${Number(item.id)}" alt="识别时间 ${esc(item.recognized_at)}" loading="lazy">
      </article>
    `).join('');
  } catch (e) {
    error.textContent = e.message || '读取当天识别照片失败。';
    error.classList.remove('hidden');
  }
}

function closeAttendanceSnapshot() {
  $('#attendanceSnapshotModal').classList.add('hidden');
  $('#attendanceSnapshotTimeline').replaceChildren();
}

$('#attendanceSnapshotClose').addEventListener('click', closeAttendanceSnapshot);
$('#attendanceSnapshotModal').addEventListener('click', event => {
  if (event.target === $('#attendanceSnapshotModal')) closeAttendanceSnapshot();
});
document.addEventListener('keydown', event => {
  if (event.key === 'Escape' && !$('#attendanceSnapshotModal').classList.contains('hidden')) {
    closeAttendanceSnapshot();
  }
});
