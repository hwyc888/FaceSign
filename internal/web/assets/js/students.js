function classOptionsHTML(selected = '', includeBlank = true) {
  const blank = includeBlank ? '<option value="">请选择班级</option>' : '';
  return blank + classesCache.map(item =>
    '<option value="' + esc(item.name) + '" ' + (item.name === selected ? 'selected' : '') + '>' + esc(item.name) + '</option>'
  ).join('');
}

async function loadStudents() {
  try {
    if (!classesCache.length) await loadClasses();
    const items = await api('/api/students');
    studentsCache = items;
    $('#studentsBody').innerHTML = items.map(s => `
      <tr>
        <td>${esc(s.student_no)}</td>
        <td>${esc(s.name)}</td>
        <td><select class="table-class-select" data-student-class="${s.id}">${classOptionsHTML(s.class_name, false)}</select></td>
        <td><input class="seat-number-input" data-student-seat="${s.id}" type="number" min="0" value="${s.seat_no || ''}" placeholder="未编排"></td>
        <td><span class="badge ${s.face_count > 0 ? 'yes' : 'no'}">${s.face_count > 0 ? s.face_count + ' 个样本' : '未录入'}</span></td>
        <td>
          <div class="student-actions">
            <button data-faces="${s.id}">人脸样本</button>
            <button class="danger" data-del="${s.id}">删除</button>
          </div>
        </td>
      </tr>
    `).join('') || '<tr><td colspan="6">暂无学生</td></tr>';

    $$('[data-student-class]').forEach(select => {
      select.onchange = async () => {
        const studentID = Number(select.dataset.studentClass);
        const previous = studentsCache.find(s => Number(s.id) === studentID)?.class_name || '';
        try {
          await api('/api/students/' + studentID, {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({class_name: select.value})
          });
          const student = studentsCache.find(s => Number(s.id) === studentID);
          if (student) student.class_name = select.value;
          if (samplesStudent && Number(samplesStudent.id) === studentID) {
            samplesStudent.class_name = select.value;
            $('#sampleStudentClass').textContent = select.value;
          }
          toast('班级已更新；跨班时原座位号会自动清空');
          await loadStudents();
          if (typeof loadCheckinSeatBoard === 'function') await loadCheckinSeatBoard();
        } catch (e) {
          select.value = previous;
          toast(e.message);
        }
      };
    });

    $('[data-student-seat]').forEach(input => {
      input.onchange = async () => {
        const studentID = Number(input.dataset.studentSeat);
        const student = studentsCache.find(s => Number(s.id) === studentID);
        const previous = student ? Number(student.seat_no || 0) : 0;
        const seatNo = input.value.trim() === '' ? 0 : Number(input.value);
        try {
          await api('/api/students/' + studentID, {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify({seat_no: seatNo})
          });
          if (student) student.seat_no = seatNo;
          input.value = seatNo > 0 ? String(seatNo) : '';
          toast(seatNo > 0 ? ('座位号已设为 ' + seatNo) : '座位号已清空');
          if (typeof loadCheckinSeatBoard === 'function') await loadCheckinSeatBoard();
        } catch (e) {
          input.value = previous > 0 ? String(previous) : '';
          toast(e.message);
        }
      };
    });

    $('[data-faces]').forEach(button => {
      button.onclick = () => openSamplesPanel(Number(button.dataset.faces));
    });
    $$('[data-del]').forEach(button => {
      button.onclick = async () => {
        if (!confirm('确定删除该学生及其全部人脸样本、考勤数据？')) return;
        try {
          const deletedID = Number(button.dataset.del);
          await api(`/api/students/${deletedID}`, {method: 'DELETE'});
          if (samplesStudent && Number(samplesStudent.id) === deletedID) clearSampleSelection();
          await loadStudents();
          await loadHealth();
        } catch (e) {
          toast(e.message);
        }
      };
    });
  } catch (e) {
    toast(e.message);
  }
}

function renderSampleChecklist(samples) {
  const expected = ['正面', '左侧', '右侧', '轻微抬头', '轻微低头'];
  const labels = new Set(samples.map(sample => sample.label || '补充'));
  $('#sampleChecklist').innerHTML = expected.map(label =>
    `<span class="angle-chip ${labels.has(label) ? 'done' : 'pending'}">${esc(label)} ${labels.has(label) ? '✓' : ''}</span>`
  ).join('');
}

function renderSamplePanelEmpty() {
  samplesStudent = null;
  $('#samplePanelTitle').textContent = '人脸样本';
  $('#samplePanelMeta').textContent = '选择下方学生即可在这里查看和补充样本。';
  $('#sampleStudentCard').classList.add('hidden');
  $('#clearSampleSelection').classList.add('hidden');
  $('#sampleControls').classList.add('hidden');
  $('#sampleCount').textContent = '0';
  renderSampleChecklist([]);
  $('#samplesList').innerHTML = '<div class="sample-empty">还未选择学生。首次录入可直接在左侧拍照；已有学生可从下方列表进入样本管理。</div>';
}

async function openSamplesPanel(studentID, options = {}) {
  const student = studentsCache.find(s => Number(s.id) === Number(studentID)) ||
    (duplicateStudent && Number(duplicateStudent.id) === Number(studentID) ? duplicateStudent : null);
  if (!student) {
    await loadStudents();
    return openSamplesPanel(studentID, options);
  }

  samplesStudent = student;
  $('#samplePanelTitle').textContent = '学生人脸样本';
  $('#samplePanelMeta').textContent = '可直接查看完成角度、删除单个样本或继续补充。';
  $('#sampleStudentName').textContent = student.name;
  $('#sampleStudentNo').textContent = student.student_no;
  $('#sampleStudentClass').textContent = student.class_name || '-';
  $('#sampleStudentCard').classList.remove('hidden');
  $('#clearSampleSelection').classList.remove('hidden');
  $('#sampleControls').classList.remove('hidden');

  try {
    const samples = await api(`/api/students/${student.id}/faces`);
    $('#sampleCount').textContent = String(samples.length);
    renderSampleChecklist(samples);
    $('#samplesList').innerHTML = samples.map(sample => `
      <div class="sample-card">
        <div class="sample-card-main">
          <strong>${esc(sample.label || '补充')}</strong>
          <span>${esc(sample.created_at)}</span>
        </div>
        <button class="danger sample-delete" data-delete-sample="${sample.id}">删除</button>
      </div>
    `).join('') || '<div class="sample-empty">该学生还没有人脸样本，可以从下方选择角度开始补充。</div>';

    $$('[data-delete-sample]').forEach(button => {
      button.onclick = async () => {
        if (!confirm('确定删除这个人脸样本？')) return;
        try {
          await api(`/api/students/${student.id}/faces/${button.dataset.deleteSample}`, {method: 'DELETE'});
          await loadStudents();
          await openSamplesPanel(student.id, {scroll: false});
        } catch (e) {
          toast(e.message);
        }
      };
    });

    if (options.scroll !== false) {
      $('#enrollmentWorkbench').scrollIntoView({behavior: 'smooth', block: 'start'});
    }
  } catch (e) {
    toast(e.message);
  }
}

$('#startSupplement').addEventListener('click', async () => {
  if (!samplesStudent) return;
  supplementStudent = samplesStudent;
  const angle = $('#sampleAngle').value;
  $('#enrollMode').textContent = `补充：${supplementStudent.name}`;
  $('#cancelSupplement').classList.remove('hidden');
  $('#captureEnrollment').textContent = '拍照补充样本';
  setEnrollmentAngle(angle);
  setEnrollmentStatus(
    `正在补充 ${supplementStudent.name}`,
    `请调整为“${angle}”角度，并确保镜头前只有该学生。`,
    'working'
  );
  await startCamera();
  $('#enrollmentWorkbench').scrollIntoView({behavior: 'smooth', block: 'start'});
});

$('#sampleAngle').addEventListener('change', () => {
  if (!supplementStudent) return;
  const angle = $('#sampleAngle').value;
  setEnrollmentAngle(angle);
  setEnrollmentStatus(
    `正在补充 ${supplementStudent.name}`,
    `请调整为“${angle}”角度，再点击拍照补充样本。`,
    'working'
  );
});

function cancelSupplementMode(resetStatus = true) {
  supplementStudent = null;
  $('#enrollMode').textContent = '首次录入';
  $('#cancelSupplement').classList.add('hidden');
  $('#captureEnrollment').textContent = '拍照并录入';
  setEnrollmentAngle('正面');
  if (resetStatus) {
    setEnrollmentStatus('准备采集', '首次录入请正对摄像头；已有学生可从右侧选择角度继续补充。', 'neutral');
  }
}

function clearSampleSelection() {
  cancelSupplementMode();
  renderSamplePanelEmpty();
}

$('#clearSampleSelection').addEventListener('click', clearSampleSelection);
$('#cancelSupplement').addEventListener('click', () => cancelSupplementMode());
$('#refreshStudents').addEventListener('click', loadStudents);

renderSamplePanelEmpty();
