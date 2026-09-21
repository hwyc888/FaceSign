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
        <td>${esc(a.student_no)}</td>
        <td>${esc(a.name)}</td>
        <td>${esc(a.class_name || '-')}</td>
        <td>${(a.similarity * 100).toFixed(1)}%</td>
      </tr>
    `).join('') || '<tr><td colspan="5">暂无记录</td></tr>';
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
