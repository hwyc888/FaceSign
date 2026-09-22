window.classSeatEditorID = 0;
let classSeatEditorStudents = [];
let classSeatEditorSelectedStudentID = 0;
let classSeatEditorDraggingStudentID = 0;

function classSeatEditorClass() {
  return classesCache.find(item => Number(item.id) === Number(window.classSeatEditorID)) || null;
}

async function openClassSeatEditor(classID) {
  const item = classesCache.find(row => Number(row.id) === Number(classID));
  if (!item) {
    toast('班级不存在，请刷新后重试');
    return;
  }
  window.classSeatEditorID = Number(classID);
  classSeatEditorSelectedStudentID = 0;
  $('#classSeatEditor').classList.remove('hidden');
  $('#seatEditorTitle').textContent = item.name + ' · 座位编排';
  $('#seatEditorName').value = item.name;
  $('#seatEditorRows').value = item.seat_rows || 6;
  $('#seatEditorPerRow').value = item.seats_per_row || 8;
  $('#seatEditorLateAfter').value = item.late_after || '';
  $('#seatEditorDeadline').value = item.deadline || '';
  await reloadClassSeatEditor();
  $('#classSeatEditor').scrollIntoView({behavior: 'smooth', block: 'start'});
}

function closeClassSeatEditor() {
  window.classSeatEditorID = 0;
  classSeatEditorStudents = [];
  classSeatEditorSelectedStudentID = 0;
  $('#classSeatEditor').classList.add('hidden');
}

async function reloadClassSeatEditor() {
  const item = classSeatEditorClass();
  if (!item) return;
  try {
    const students = await api('/api/students');
    classSeatEditorStudents = students.filter(student => student.class_name === item.name);
    renderClassSeatEditor();
  } catch (e) {
    toast(e.message);
  }
}

function selectedSeatEditorStudent() {
  return classSeatEditorStudents.find(student =>
    Number(student.id) === Number(classSeatEditorSelectedStudentID)
  ) || null;
}

function setSeatEditorSelection(studentID) {
  classSeatEditorSelectedStudentID = Number(studentID || 0);
  renderClassSeatEditor();
}

function renderClassSeatEditor() {
  const item = classSeatEditorClass();
  if (!item) return;

  const rows = Number(item.seat_rows || 0);
  const perRow = Number(item.seats_per_row || 0);
  const selected = selectedSeatEditorStudent();
  const bySeat = new Map();
  const unassigned = [];
  classSeatEditorStudents.forEach(student => {
    if (Number(student.seat_no) > 0) {
      bySeat.set(Number(student.seat_no), student);
    } else {
      unassigned.push(student);
    }
  });

  const rowHTML = [];
  for (let row = 0; row < rows; row++) {
    const seats = [];
    for (let col = 0; col < perRow; col++) {
      const seatNo = row * perRow + col + 1;
      const student = bySeat.get(seatNo);
      const selectedClass = student && selected && Number(student.id) === Number(selected.id) ? ' selected' : '';
      if (student) {
        seats.push(`<button class="seat-editor-cell occupied${selectedClass}" type="button"
          draggable="true" data-editor-seat="${seatNo}" data-editor-student-id="${student.id}">
          <span class="seat-no">${seatNo}号</span>
          <strong>${esc(student.name)}</strong>
          <span>${esc(student.student_no)}</span>
          <span class="drag-hint">拖动调整位置</span>
        </button>`);
      } else {
        seats.push(`<button class="seat-editor-cell empty" type="button" data-editor-seat="${seatNo}">
          <span class="seat-no">${seatNo}号</span>
          <strong>空位</strong>
          <span>点击作为目标座位</span>
        </button>`);
      }
    }
    rowHTML.push(`<div class="seat-editor-row">
      <div class="seat-row-label">第${row + 1}排</div>
      <div class="seat-editor-row-cells" style="--seats-per-row:${perRow}">${seats.join('')}</div>
    </div>`);
  }
  $('#seatEditorBoard').innerHTML = rowHTML.join('') ||
    '<div class="seat-board-empty">请先设置有效的排数和每排人数。</div>';

  $('#seatEditorUnassigned').innerHTML = unassigned.length
    ? '<strong>未编座位：</strong>' + unassigned.map(student => {
        const active = selected && Number(selected.id) === Number(student.id) ? ' selected' : '';
        return `<button class="unassigned-student${active}" type="button"
          draggable="true" data-editor-student="${student.id}" data-editor-student-id="${student.id}">
          ${esc(student.name)} · ${esc(student.student_no)}
        </button>`;
      }).join('')
    : '<span class="muted">全部学生都已编排座位。</span>';

  const selection = $('#seatEditorSelection');
  const clear = $('#seatEditorClearSeat');
  if (selected) {
    selection.textContent = selected.seat_no > 0
      ? `已选择：${selected.name}（当前 ${selected.seat_no}号）→ 再点一个目标座位即可移动/交换`
      : `已选择：${selected.name}（未编座位）→ 点击目标座位进行安排`;
    clear.disabled = Number(selected.seat_no || 0) === 0;
  } else {
    selection.textContent = '尚未选择学生；先点击一个已有学生座位或下方“未编座位”学生';
    clear.disabled = true;
  }

  $$('[data-editor-seat]').forEach(button => {
    button.onclick = async () => {
      if (classSeatEditorDraggingStudentID) return;
      const targetSeatNo = Number(button.dataset.editorSeat);
      const occupant = bySeat.get(targetSeatNo);
      const current = selectedSeatEditorStudent();

      if (!current) {
        if (occupant) {
          setSeatEditorSelection(occupant.id);
        } else {
          toast('这是空位，请先选择要移动的学生，或直接拖动学生到这里');
        }
        return;
      }
      if (occupant && Number(occupant.id) === Number(current.id)) {
        setSeatEditorSelection(0);
        return;
      }
      await moveSeatEditorStudent(current.id, targetSeatNo);
    };

    button.ondragover = event => {
      if (!classSeatEditorDraggingStudentID) return;
      event.preventDefault();
      event.dataTransfer.dropEffect = 'move';
      button.classList.add('drag-over');
      const occupant = bySeat.get(Number(button.dataset.editorSeat));
      button.classList.toggle(
        'swap-target',
        Boolean(occupant && Number(occupant.id) !== Number(classSeatEditorDraggingStudentID))
      );
    };
    button.ondragleave = () => {
      button.classList.remove('drag-over', 'swap-target');
    };
    button.ondrop = async event => {
      event.preventDefault();
      const studentID = Number(
        event.dataTransfer.getData('text/plain') || classSeatEditorDraggingStudentID
      );
      const targetSeatNo = Number(button.dataset.editorSeat);
      button.classList.remove('drag-over', 'swap-target');
      if (!studentID) return;

      const moving = classSeatEditorStudents.find(student => Number(student.id) === studentID);
      if (!moving || Number(moving.seat_no || 0) === targetSeatNo) {
        classSeatEditorDraggingStudentID = 0;
        return;
      }
      classSeatEditorDraggingStudentID = 0;
      await moveSeatEditorStudent(studentID, targetSeatNo, {confirmSwap: false});
    };
  });

  $('[data-editor-student]').forEach(button => {
    button.onclick = () => {
      if (!classSeatEditorDraggingStudentID) {
        setSeatEditorSelection(Number(button.dataset.editorStudent));
      }
    };
  });

  $('[data-editor-student-id]').forEach(button => {
    button.ondragstart = event => {
      const studentID = Number(button.dataset.editorStudentId);
      if (!studentID) {
        event.preventDefault();
        return;
      }
      classSeatEditorDraggingStudentID = studentID;
      classSeatEditorSelectedStudentID = studentID;
      event.dataTransfer.effectAllowed = 'move';
      event.dataTransfer.setData('text/plain', String(studentID));
      button.classList.add('dragging');
      const selectedStudent = classSeatEditorStudents.find(student => Number(student.id) === studentID);
      if (selectedStudent) {
        $('#seatEditorSelection').textContent = selectedStudent.seat_no > 0
          ? `正在拖动：${selectedStudent.name}（${selectedStudent.seat_no}号）`
          : `正在拖动：${selectedStudent.name}（未编座位）`;
      }
    };
    button.ondragend = () => {
      classSeatEditorDraggingStudentID = 0;
      button.classList.remove('dragging');
      $$('.seat-editor-cell').forEach(cell => cell.classList.remove('drag-over', 'swap-target'));
    };
  });
}

async function moveSeatEditorStudent(studentID, targetSeatNo, options = {}) {
  const item = classSeatEditorClass();
  if (!item) return;
  try {
    const targetStudent = classSeatEditorStudents.find(student => Number(student.seat_no) === Number(targetSeatNo));
    if (targetStudent && Number(targetStudent.id) !== Number(studentID) && options.confirmSwap !== false) {
      const moving = classSeatEditorStudents.find(student => Number(student.id) === Number(studentID));
      if (!confirm(`${moving ? moving.name : '所选学生'} 移到 ${targetSeatNo}号后，将与 ${targetStudent.name} 交换座位。继续吗？`)) {
        return;
      }
    }
    await api(`/api/classes/${item.id}/seats`, {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({student_id: Number(studentID), target_seat_no: Number(targetSeatNo)})
    });
    classSeatEditorSelectedStudentID = 0;
    await reloadClassSeatEditor();
    if (typeof loadStudents === 'function') await loadStudents();
    if (typeof loadCheckinSeatBoard === 'function') await loadCheckinSeatBoard();
    toast(targetStudent ? '座位已交换' : '座位已移动');
  } catch (e) {
    toast(e.message);
  }
}

$('#closeSeatEditor').addEventListener('click', closeClassSeatEditor);

$('#seatEditorClearSeat').addEventListener('click', async () => {
  const selected = selectedSeatEditorStudent();
  if (!selected) return;
  if (!confirm(`清空 ${selected.name} 的座位号，让该位置变成空位？`)) return;
  await moveSeatEditorStudent(selected.id, 0);
});

$('#seatEditorAutoArrange').addEventListener('click', async () => {
  const item = classSeatEditorClass();
  if (!item) return;
  if (!confirm(`将“${item.name}”按学号顺序重新编排座位？现有座位调整会被覆盖。`)) return;
  try {
    const result = await api(`/api/classes/${item.id}/arrange`, {method: 'POST'});
    classSeatEditorSelectedStudentID = 0;
    toast(`已重新编排 ${result.arranged} 名学生`);
    await reloadClassSeatEditor();
    if (typeof loadStudents === 'function') await loadStudents();
    if (typeof loadCheckinSeatBoard === 'function') await loadCheckinSeatBoard();
  } catch (e) {
    toast(e.message);
  }
});

$('#seatEditorSettingsForm').addEventListener('submit', async e => {
  e.preventDefault();
  const item = classSeatEditorClass();
  if (!item) return;

  try {
    const updated = await api(`/api/classes/${item.id}/settings`, {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        name: $('#seatEditorName').value.trim(),
        seat_rows: Number($('#seatEditorRows').value),
        seats_per_row: Number($('#seatEditorPerRow').value),
        late_after: $('#seatEditorLateAfter').value,
        deadline: $('#seatEditorDeadline').value
      })
    });
    toast('班级与考勤时间设置已保存');
    await loadClasses();
    await openClassSeatEditor(updated.id);
    if (typeof loadStudents === 'function') await loadStudents();
    if (typeof loadCheckinSeatBoard === 'function') await loadCheckinSeatBoard();
  } catch (e) {
    toast(e.message);
  }
});
