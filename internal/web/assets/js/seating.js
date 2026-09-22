let seatBoardFilter = 'all';

function renderCheckinSeatClassOptions() {
  const select = $('#checkinSeatClass');
  if (!select) return;
  const current = select.value;
  select.innerHTML = '<option value="">请选择班级</option>' +
    classesCache.map(item => `<option value="${esc(item.name)}">${esc(item.name)}</option>`).join('');
  if (classesCache.some(item => item.name === current)) {
    select.value = current;
  } else if (classesCache.length) {
    select.value = classesCache[0].name;
  }
}

function seatSummaryHTML(board) {
  const rate = board.total > 0 ? ((board.signed / board.total) * 100).toFixed(1) : '0.0';
  return [
    `<button class="seat-stat filter ${seatBoardFilter === 'all' ? 'active' : ''}" data-seat-filter="all">全部学生 <strong>${board.total}</strong></button>`,
    `<button class="seat-stat signed filter ${seatBoardFilter === 'signed' ? 'active' : ''}" data-seat-filter="signed">正常签到 <strong>${board.on_time || 0}</strong></button>`,
    `<button class="seat-stat late filter ${seatBoardFilter === 'late' ? 'active' : ''}" data-seat-filter="late">迟到签到 <strong>${board.late || 0}</strong></button>`,
    `<button class="seat-stat waiting filter ${seatBoardFilter === 'waiting' ? 'active' : ''}" data-seat-filter="waiting">待签到 <strong>${board.waiting || 0}</strong></button>`,
    `<button class="seat-stat absent filter ${seatBoardFilter === 'absent' ? 'active' : ''}" data-seat-filter="absent">截止未签 <strong>${board.absent || 0}</strong></button>`,
    `<button class="seat-stat empty filter ${seatBoardFilter === 'empty' ? 'active' : ''}" data-seat-filter="empty">空位 <strong>${board.empty_seats || 0}</strong></button>`,
    `<span class="seat-stat rate">签到率 <strong>${rate}%</strong></span>`
  ].join('');
}

function attendanceStatusLabel(status, checkedAt) {
  switch (status) {
    case 'signed': return checkedAt ? ('正常签到 ' + checkedAt) : '正常签到';
    case 'late': return checkedAt ? ('迟到签到 ' + checkedAt) : '迟到签到';
    case 'absent': return '截止时间已到 · 未签到';
    default: return '待签到';
  }
}

function renderCheckinSeatBoard(board) {
  const container = $('#checkinSeatBoard');
  const summary = $('#checkinSeatSummary');
  const unassigned = $('#checkinSeatUnassigned');
  summary.innerHTML = seatSummaryHTML(board);

  $$('[data-seat-filter]').forEach(button => {
    button.onclick = () => {
      seatBoardFilter = button.dataset.seatFilter || 'all';
      applySeatBoardFilter();
      $$('.seat-stat.filter').forEach(item =>
        item.classList.toggle('active', item.dataset.seatFilter === seatBoardFilter)
      );
    };
  });

  const rows = Number(board.class.seat_rows || 0);
  const perRow = Number(board.class.seats_per_row || 0);
  if (!rows || !perRow) {
    container.innerHTML = '<div class="seat-board-empty">该班级还没有配置座位布局。</div>';
    unassigned.classList.add('hidden');
    return;
  }

  const bySeat = new Map();
  const noSeat = [];
  (board.students || []).forEach(student => {
    if (student.seat_no > 0) bySeat.set(Number(student.seat_no), student);
    else noSeat.push(student);
  });

  const rowHTML = [];
  for (let row = 0; row < rows; row++) {
    const cells = [];
    for (let col = 0; col < perRow; col++) {
      const seatNo = row * perRow + col + 1;
      const student = bySeat.get(seatNo);
      if (!student) {
        cells.push(`<div class="seat-cell empty" data-seat-status="empty">
          <span class="seat-no">${seatNo}号</span>
          <strong>空位</strong>
          <span class="seat-status">暂无学生</span>
        </div>`);
        continue;
      }
      const state = student.status || (student.signed ? 'signed' : 'waiting');
      cells.push(`<div class="seat-cell ${state}" data-seat-status="${state}">
        <span class="seat-no">${seatNo}号</span>
        <strong>${esc(student.name)}</strong>
        <span class="seat-student-no">${esc(student.student_no)}</span>
        <span class="seat-status">${attendanceStatusLabel(state, esc(student.checked_at || ''))}</span>
      </div>`);
    }
    rowHTML.push(`<div class="seat-row">
      <div class="seat-row-label">第${row + 1}排</div>
      <div class="seat-row-cells" style="--seats-per-row:${perRow}">${cells.join('')}</div>
    </div>`);
  }

  const ruleText = board.class.late_after || board.class.deadline
    ? `迟到：${esc(board.class.late_after || '未设置')}　截止：${esc(board.class.deadline || '未设置')}`
    : '尚未设置迟到时间和签到截止时间';
  container.innerHTML =
    `<div class="classroom-stage checkin-stage">讲台 / 黑板（前方）<span>${ruleText}</span></div>` +
    rowHTML.join('');

  if (noSeat.length) {
    unassigned.innerHTML = '<strong>未编座位：</strong>' + noSeat.map(student => {
      const state = student.status || (student.signed ? 'signed' : 'waiting');
      return `<span class="unassigned-chip ${state}" data-seat-status="${state}">
        ${esc(student.name)} · ${attendanceStatusLabel(state, esc(student.checked_at || ''))}
      </span>`;
    }).join('');
    unassigned.classList.remove('hidden');
  } else {
    unassigned.classList.add('hidden');
    unassigned.innerHTML = '';
  }
  applySeatBoardFilter();
}

function applySeatBoardFilter() {
  $$('[data-seat-status]').forEach(item => {
    const match = seatBoardFilter === 'all' || item.dataset.seatStatus === seatBoardFilter;
    item.classList.toggle('filtered-out', !match);
  });
}

async function loadCheckinSeatBoard() {
  const select = $('#checkinSeatClass');
  if (!select) return;
  if (!select.value) renderCheckinSeatClassOptions();
  const className = select.value;
  if (!className) {
    $('#checkinSeatSummary').innerHTML = '<span class="seat-stat">暂无班级</span>';
    $('#checkinSeatBoard').innerHTML = '<div class="seat-board-empty">请先在设置中建立班级。</div>';
    return;
  }
  try {
    const board = await api('/api/attendance/seats?day=' + encodeURIComponent(today()) + '&class_name=' + encodeURIComponent(className));
    renderCheckinSeatBoard(board);
  } catch (e) {
    $('#checkinSeatBoard').innerHTML = '<div class="seat-board-empty">' + esc(e.message) + '</div>';
  }
}

$('#checkinSeatClass').addEventListener('change', () => {
  seatBoardFilter = 'all';
  loadCheckinSeatBoard();
});

setInterval(() => {
  const select = $('#checkinSeatClass');
  if (select && select.value) loadCheckinSeatBoard();
}, 30000);
