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

    const body = $('#classesBody');
    if (!body) return;
    body.innerHTML = items.map((item, index) => `
      <tr>
        <td>${index + 1}</td>
        <td><strong>${esc(item.name)}</strong></td>
        <td>${item.student_count} 人</td>
        <td>
          <div class="class-order-actions">
            <button data-class-up="${item.id}" ${index === 0 ? 'disabled' : ''}>上移</button>
            <button data-class-down="${item.id}" ${index === items.length - 1 ? 'disabled' : ''}>下移</button>
          </div>
        </td>
        <td>
          <div class="student-actions">
            <button data-class-rename="${item.id}">改名</button>
            <button class="danger" data-class-delete="${item.id}">删除</button>
          </div>
        </td>
      </tr>
    `).join('') || '<tr><td colspan="5">还没有班级，请先添加</td></tr>';

    $$('[data-class-up]').forEach(button => {
      button.onclick = () => moveClass(Number(button.dataset.classUp), 'up');
    });
    $$('[data-class-down]').forEach(button => {
      button.onclick = () => moveClass(Number(button.dataset.classDown), 'down');
    });
    $$('[data-class-rename]').forEach(button => {
      button.onclick = async () => {
        const item = classesCache.find(c => Number(c.id) === Number(button.dataset.classRename));
        if (!item) return;
        const name = prompt('请输入新的班级名称', item.name);
        if (name === null || name.trim() === item.name) return;
        try {
          await api(`/api/classes/${item.id}`, {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({name: name.trim()})
          });
          toast('班级名称已修改，学生资料已同步更新');
          await loadClasses();
          await loadStudents();
        } catch (e) {
          toast(e.message);
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
  if (!name) return;
  try {
    await api('/api/classes', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({name})
    });
    input.value = '';
    toast('班级已添加');
    await loadClasses();
  } catch (err) {
    toast(err.message);
  }
});
