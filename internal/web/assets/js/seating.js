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
    `<span class="seat-stat"><strong>${esc(board.class.name)}</strong></span>`,
    `<span class="seat-stat">总人数 <strong>${board.total}</strong></span>`,
    `<span class="seat-stat signed">已签到 <strong>${board.signed}</strong></span>`,
    `<span class="seat-stat unsigned">未签到 <strong>${board.unsigned}</strong></span>`,
    `<span class="seat-stat">签到率 <strong>${rate}%</strong></span>`
  ].join('');
}

function renderCheckinSeatBoard(board) {
  const container = $('#checkinSeatBoard');
  const summary = $('#checkinSeatSummary');
  const unassigned = $('#checkinSeatUnassigned');
  summary.innerHTML = seatSummaryHTML(board);

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
        cells.push(`<div class="seat-cell empty"><span class="seat-no">${seatNo}号</span><strong>空位</strong></div>`);
        continue;
      }
      const state = student.signed ? 'signed' : 'unsigned';
      const status = student.signed ? ('已签到 ' + esc(student.checked_at || '')) : '未签到';
      cells.push(`<div class="seat-cell ${state}">
        <span class="seat-no">${seatNo}号</span>
        <strong>${esc(student.name)}</strong>
        <span class="seat-student-no">${esc(student.student_no)}</span>
        <span class="seat-status">${status}</span>
      </div>`);
    }
    rowHTML.push(`<div class="seat-row">
      <div class="seat-row-label">第${row + 1}排</div>
      <div class="seat-row-cells" style="--seats-per-row:${perRow}">${cells.join('')}</div>
    </div>`);
  }
  container.innerHTML = rowHTML.join('');

  if (noSeat.length) {
    unassigned.innerHTML = '<strong>未编座位：</strong>' + noSeat.map(student =>
      `<span class="unassigned-chip ${student.signed ? 'signed' : 'unsigned'}">${esc(student.name)} · ${student.signed ? '已签到' : '未签到'}</span>`
    ).join('');
    unassigned.classList.remove('hidden');
  } else {
    unassigned.classList.add('hidden');
    unassigned.innerHTML = '';
  }
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

$('#checkinSeatClass').addEventListener('change', loadCheckinSeatBoard);
