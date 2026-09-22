function renderClassOptions(selected = '') {
  const select = $('#modalClassName');
  if (!select) return;
  select.innerHTML = '<option value="">请选择班级</option>' +
    classesCache.map(item => `<option value="${esc(item.name)}">${esc(item.name)}</option>`).join('');
  if (selected) select.value = selected;
}

async function loadClasses() {
  try {
    const items = await api('/api/classes');
    classesCache = items;
    renderClassOptions($('#modalClassName') ? $('#modalClassName').value : '');
    if (typeof renderCheckinSeatClassOptions === 'function') renderCheckinSeatClassOptions();

    const body = $('#classesBody');
    if (!body) return;
    body.innerHTML = items.map((item, index) => `
      <tr>
        <td>${index + 1}</td>
        <td><strong>${esc(item.name)}</strong></td>
        <td>${item.student_count} 人</td>
        <td>${item.seat_rows || 6} 排 × ${item.seats_per_row || 8} 人（${(item.seat_rows || 6) * (item.seats_per_row || 8)} 座）</td>
        <td>${esc(item.late_after || '未设置')}</td>
        <td>${esc(item.deadline || '未设置')}</td>
        <td>
          <div class="class-order-actions">
            <button data-class-up="${item.id}" ${index === 0 ? 'disabled' : ''}>上移</button>
            <button data-class-down="${item.id}" ${index === items.length - 1 ? 'disabled' : ''}>下移</button>
          </div>
        </td>
        <td>
          <div class="student-actions">
            <button class="primary-soft" data-class-edit="${item.id}">编辑 / 编排</button>
            <button class="danger" data-class-delete="${item.id}">删除</button>
          </div>
        </td>
      </tr>
    `).join('') || '<tr><td colspan="8">还没有班级，请先添加</td></tr>';

    $$('[data-class-up]').forEach(button => {
      button.onclick = () => moveClass(Number(button.dataset.classUp), 'up');
    });
    $$('[data-class-down]').forEach(button => {
      button.onclick = () => moveClass(Number(button.dataset.classDown), 'down');
    });
    $$('[data-class-edit]').forEach(button => {
      button.onclick = () => {
        if (typeof openClassSeatEditor === 'function') {
          openClassSeatEditor(Number(button.dataset.classEdit));
        }
      };
    });
    $$('[data-class-delete]').forEach(button => {
      button.onclick = async () => {
        const item = classesCache.find(c => Number(c.id) === Number(button.dataset.classDelete));
        if (!item) return;
        if (!confirm(`确定删除班级“${item.name}”？只有没有学生的班级才能删除。`)) return;
        try {
          await api(`/api/classes/${item.id}`, {method: 'DELETE'});
          toast('班级已删除');
          if (typeof closeClassSeatEditor === 'function' && Number(window.classSeatEditorID || 0) === Number(item.id)) {
            closeClassSeatEditor();
          }
          await loadClasses();
        } catch (e) {
          toast(e.message);
        }
      };
    });
  } catch (e) {
    toast(e.message);
  }
}

async function moveClass(id, direction) {
  try {
    await api(`/api/classes/${id}/move`, {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({direction})
    });
    await loadClasses();
  } catch (e) {
    toast(e.message);
  }
}

$('#classForm').addEventListener('submit', async e => {
  e.preventDefault();
  const input = $('#className');
  const name = input.value.trim();
  const seatRows = Number($('#classSeatRows').value);
  const seatsPerRow = Number($('#classSeatsPerRow').value);
  const lateAfter = $('#classLateAfter').value;
  const deadline = $('#classDeadline').value;
  if (!name) return;
  try {
    await api('/api/classes', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        name,
        seat_rows: seatRows,
        seats_per_row: seatsPerRow,
        late_after: lateAfter,
        deadline
      })
    });
    input.value = '';
    $('#classLateAfter').value = '';
    $('#classDeadline').value = '';
    toast('班级已添加，可继续进入“编辑 / 编排”调整座位');
    await loadClasses();
  } catch (err) {
    toast(err.message);
  }
});
