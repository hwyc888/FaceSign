async function loadStudents() {
  try {
    const items = await api('/api/students');
    studentsCache = items;
    $('#studentsBody').innerHTML = items.map(s => `
      <tr>
        <td>${esc(s.student_no)}</td>
        <td>${esc(s.name)}</td>
        <td>${esc(s.class_name || '-')}</td>
        <td><span class="badge ${s.face_count > 0 ? 'yes' : 'no'}">${s.face_count > 0 ? s.face_count + ' 个样本' : '未录入'}</span></td>
        <td>
          <div class="student-actions">
            <button data-faces="${s.id}">人脸样本</button>
            <button class="danger" data-del="${s.id}">删除</button>
          </div>
        </td>
      </tr>
    `).join('') || '<tr><td colspan="5">暂无学生</td></tr>';

    $$('[data-faces]').forEach(button => {
      button.onclick = () => openSamplesModal(Number(button.dataset.faces));
    });
    $$('[data-del]').forEach(button => {
      button.onclick = async () => {
        if (!confirm('确定删除该学生及其全部人脸样本、考勤数据？')) return;
        try {
          await api(`/api/students/${button.dataset.del}`, {method: 'DELETE'});
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

async function openSamplesModal(studentID) {
  const student = studentsCache.find(s => Number(s.id) === Number(studentID)) ||
    (duplicateStudent && Number(duplicateStudent.id) === Number(studentID) ? duplicateStudent : null);
  if (!student) {
    await loadStudents();
    return openSamplesModal(studentID);
  }

  samplesStudent = student;
  $('#samplesTitle').textContent = `${student.name} · 人脸样本`;
  $('#samplesHint').textContent = `${student.student_no} | ${student.class_name || '-'} · 可查询、删除或继续补充多角度样本`;
  openModal('samplesModal');

  try {
    const samples = await api(`/api/students/${student.id}/faces`);
    $('#samplesBody').innerHTML = samples.map(sample => `
      <tr>
        <td>${esc(sample.label || '补充')}</td>
        <td>${esc(sample.created_at)}</td>
        <td><button class="danger" data-delete-sample="${sample.id}">删除样本</button></td>
      </tr>
    `).join('') || '<tr><td colspan="3">该学生还没有人脸样本</td></tr>';

    $$('[data-delete-sample]').forEach(button => {
      button.onclick = async () => {
        if (!confirm('确定删除这个人脸样本？')) return;
        try {
          await api(`/api/students/${student.id}/faces/${button.dataset.deleteSample}`, {method: 'DELETE'});
          await loadStudents();
          await openSamplesModal(student.id);
        } catch (e) {
          toast(e.message);
        }
      };
    });
  } catch (e) {
    toast(e.message);
  }
}

$('#startSupplement').addEventListener('click', async () => {
  if (!samplesStudent) return;
  supplementStudent = samplesStudent;
  $('#supplementAngle').value = $('#sampleAngle').value;
  closeModal('samplesModal');
  $('#enrollMode').textContent = `补充：${supplementStudent.name}`;
  $('#supplementAngleWrap').classList.remove('hidden');
  $('#cancelSupplement').classList.remove('hidden');
  $('#captureEnrollment').textContent = '拍照补充';
  $('#enrollInstruction').innerHTML = `
    <strong>正在补充 ${esc(supplementStudent.name)} 的人脸样本</strong>
    <span>请选择角度，并确保镜头前只有该学生。</span>
    <span>系统会先与该学生已有样本做身份连续性校验，同时检查是否误录成其他学生。</span>
    <span>过于相似的重复角度会被拒绝，请适当改变朝向。</span>
  `;
  await startCamera();
});

function cancelSupplementMode() {
  supplementStudent = null;
  $('#enrollMode').textContent = '首次录入';
  $('#supplementAngleWrap').classList.add('hidden');
  $('#cancelSupplement').classList.add('hidden');
  $('#captureEnrollment').textContent = '拍照并录入';
  $('#enrollInstruction').innerHTML = `
    <strong>录入步骤</strong>
    <span>1. 正对摄像头并让脸部位于引导框内。</span>
    <span>2. 点击“拍照并录入”，系统先检查是否已经录入。</span>
    <span>3. 确认新面孔后，再填写学号、姓名和班级。</span>
  `;
}

$('#cancelSupplement').addEventListener('click', cancelSupplementMode);
$('#refreshStudents').addEventListener('click', loadStudents);
