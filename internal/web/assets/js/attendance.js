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
    $('#attendanceBody').innerHTML = items.map(a => `
      <tr>
        <td>${esc(a.checked_at)}</td>
        <td>${esc(a.last_seen_at || a.checked_at)}</td>
        <td>${Number(a.recognition_count || 1)} 次</td>
        <td>${esc(a.student_no)}</td>
        <td>${esc(a.name)}</td>
        <td>${esc(a.class_name || '-')}</td>
        <td>${(a.similarity * 100).toFixed(1)}%</td>
        <td>${a.snapshot_available
          ? (typeof currentUser !== 'undefined' && currentUser && currentUser.is_admin
            ? '<button type="button" class="small" data-view-attendance-snapshot="' + Number(a.id) + '">查看截图</button>'
            : '<span class="muted">管理员可查看</span>')
          : '<span class="muted">暂无截图</span>'}</td>
      </tr>
    `).join('') || '<tr><td colspan="8">暂无记录</td></tr>';
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
attendanceBody.addEventListener('click', event => {
  const button = event.target.closest('[data-view-attendance-snapshot]');
  if (!button) return;
  const attendanceID = Number(button.dataset.viewAttendanceSnapshot);
  if (!Number.isSafeInteger(attendanceID) || attendanceID <= 0) return;
  const modal = $('#attendanceSnapshotModal');
  const image = $('#attendanceSnapshotImage');
  $('#attendanceSnapshotTitle').textContent = '识别现场截图 · 考勤记录 #' + attendanceID;
  $('#attendanceSnapshotError').classList.add('hidden');
  image.classList.remove('hidden');
  image.onerror = () => {
    image.classList.add('hidden');
    $('#attendanceSnapshotError').classList.remove('hidden');
  };
  image.src = '/api/attendance/' + attendanceID + '/snapshot?ts=' + Date.now();
  modal.classList.remove('hidden');
});

function closeAttendanceSnapshot() {
  $('#attendanceSnapshotModal').classList.add('hidden');
  $('#attendanceSnapshotImage').removeAttribute('src');
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
