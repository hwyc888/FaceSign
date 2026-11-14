let seatBoardFilter = 'all';
let lastCheckinRecognizedClass = '';
let previousMatchedStudentIDs = new Set();
let lastRenderedCheckinSeatClass = '';

async function selectCheckinSeatClassFromRecognition(result) {
  const select = $('#checkinSeatClass');
  if (!select) return false;
  const faces = Array.isArray(result?.faces) ? result.faces : [];
  const persons = Array.isArray(result?.persons) ? result.persons : [];
  const identities = new Map();
  persons.forEach(person => {
    if (!person.student) return;
    const id = String(person.student.id || person.student.student_no);
    identities.set(id, {...person, matched: true});
  });
  faces.forEach(face => {
    if (!face.student) return;
    const id = String(face.student.id || face.student.student_no);
    identities.set(id, face);
  });
  const identified = [...identities.values()].filter(face =>
    (face.recognized || face.matched) &&
    face.student &&
    face.status !== '疑似照片/屏幕'
  );
  const matched = identified.filter(face => String(face.student.class_name || '').trim());
  const ids = new Set(identified.map(face => String(face.student.id || face.student.student_no)));
  const target = matched.find(face => face.recognized && face.first_checkin_today) ||
    matched.find(face => !previousMatchedStudentIDs.has(String(face.student.id || face.student.student_no))) ||
    matched.find(face => face.student.class_name === select.value) ||
    matched[0];
  previousMatchedStudentIDs = ids;
  if (!target) {
    const missingClassStudent = identified[0];
    if (!missingClassStudent) return false;
    $('#checkinSeatSummary').innerHTML = '<span class="seat-stat">已识别 · 学生资料未设置班级</span>';
    $('#checkinSeatBoard').innerHTML =
      '<div class="seat-board-empty">已识别 ' + esc(missingClassStudent.student.name || '该学生') +
      '，但学生资料中的所属班级为空。请在“学生管理”补齐班级资料，之后座位图会自动显示。</div>';
    $('#checkinSeatUnassigned').classList.add('hidden');
    lastCheckinRecognizedClass = '';
    select.value = '';
    lastRenderedCheckinSeatClass = '';
    return true;
  }

  const className = String(target.student.class_name).trim();
  lastCheckinRecognizedClass = className;
  if (!classesCache.some(item => item.name === className)) {
    try {
      const items = await api('/api/classes');
      classesCache = Array.isArray(items) ? items : [];
      renderCheckinSeatClassOptions();
    } catch (error) {
      $('#checkinSeatSummary').innerHTML = '<span class="seat-stat">班级列表读取失败</span>';
      $('#checkinSeatBoard').innerHTML = '<div class="seat-board-empty">' + esc(error.message) + '</div>';
      return true;
    }
  }
  if (!classesCache.some(item => item.name === className)) {
    $('#checkinSeatSummary').innerHTML = '<span class="seat-stat">已识别 · 班级同步中断</span>';
    $('#checkinSeatBoard').innerHTML =
      '<div class="seat-board-empty">已识别 ' + esc(target.student.name || '该学生') +
      '，所属班级“' + esc(className) + '”尚未出现在班级列表。请刷新页面后重试；学生和签到数据未更改。</div>';
    return true;
  }

  if (select.value !== className) {
    select.value = className;
    seatBoardFilter = 'all';
    await loadCheckinSeatBoard();
    return true;
  }
  if (lastRenderedCheckinSeatClass !== className) {
    await loadCheckinSeatBoard();
    return true;
  }
  return false;
}

function renderCheckinSeatClassOptions() {
  const select = $('#checkinSeatClass');
  if (!select) return;
  const current = select.value;
  select.innerHTML = '<option value="">请选择班级</option>' +
    classesCache.map(item => `<option value="${esc(item.name)}">${esc(item.name)}</option>`).join('');
  if (classesCache.some(item => item.name === lastCheckinRecognizedClass)) {
    select.value = lastCheckinRecognizedClass;
  } else if (classesCache.some(item => item.name === current)) {
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

function attendanceStatusLabel(status, checkedAt, lastSeenAt, recognitionCount) {
  let label = '待签到';
  if (status === 'signed') label = checkedAt ? ('正常签到 ' + checkedAt) : '正常签到';
  else if (status === 'late') label = checkedAt ? ('迟到签到 ' + checkedAt) : '迟到签到';
  else if (status === 'absent') return '截止时间已到 · 未签到';

  const count = Number(recognitionCount || 0);
  if (count > 1) {
    label += ' · 最近识别 ' + (lastSeenAt || checkedAt || '-');
    label += ' · ' + count + '次';
  }
  return label;
}

function renderCheckinSeatBoard(board) {
  lastRenderedCheckinSeatClass = String(board.class?.name || '');
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
        <span class="seat-status">${attendanceStatusLabel(state, esc(student.checked_at || ''), esc(student.last_seen_at || ''), student.recognition_count)}</span>
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
        ${esc(student.name)} · ${attendanceStatusLabel(state, esc(student.checked_at || ''), esc(student.last_seen_at || ''), student.recognition_count)}
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

async function loadCheckinSeatBoard(selectedClassName = '') {
  const select = $('#checkinSeatClass');
  if (!select) return;
  let className = String(selectedClassName || select.value || '').trim();
  if (!className) {
    renderCheckinSeatClassOptions();
    className = String(select.value || '').trim();
  }
  if (!className) {
    lastRenderedCheckinSeatClass = '';
    $('#checkinSeatSummary').innerHTML = '<span class="seat-stat">暂无班级</span>';
    $('#checkinSeatBoard').innerHTML = '<div class="seat-board-empty">当前没有可选择的班级，请先在设置中创建班级。</div>';
    $('#checkinSeatUnassigned').classList.add('hidden');
    return;
  }
  try {
    const board = await api('/api/attendance/seats?day=' + encodeURIComponent(today()) + '&class_name=' + encodeURIComponent(className));
    if (select.value === className) renderCheckinSeatBoard(board);
  } catch (e) {
    if (select.value === className) {
      lastRenderedCheckinSeatClass = '';
      $('#checkinSeatSummary').innerHTML = '<span class="seat-stat">班级座位图加载失败</span>';
      $('#checkinSeatBoard').innerHTML = '<div class="seat-board-empty">' + esc(e.message) + '</div>';
    }
  }
}

async function handleCheckinSeatClassChange() {
  const select = $('#checkinSeatClass');
  if (!select) return;
  const className = String(select.value || '').trim();
  lastCheckinRecognizedClass = '';
  lastRenderedCheckinSeatClass = '';
  seatBoardFilter = 'all';
  if (!className) {
    $('#checkinSeatSummary').innerHTML = '<span class="seat-stat">请选择班级查看签到状态</span>';
    $('#checkinSeatBoard').innerHTML = '<div class="seat-board-empty">请选择班级查看座位图。</div>';
    $('#checkinSeatUnassigned').classList.add('hidden');
    return;
  }
  $('#checkinSeatSummary').innerHTML = '<span class="seat-stat">正在加载 ' + esc(className) + ' 班签到状态...</span>';
  $('#checkinSeatBoard').innerHTML = '<div class="seat-board-empty">正在加载 ' + esc(className) + ' 班座位图...</div>';
  $('#checkinSeatUnassigned').classList.add('hidden');
  await loadCheckinSeatBoard(className);
}

$('#checkinSeatClass').addEventListener('change', handleCheckinSeatClassChange);

setInterval(() => {
  const select = $('#checkinSeatClass');
  if (select && select.value) loadCheckinSeatBoard();
}, 30000);
