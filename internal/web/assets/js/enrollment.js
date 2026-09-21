function showDuplicate(data) {
  duplicateStudent = data.student || null;
  if (!duplicateStudent) {
    toast(data.error || '该人脸已经录入');
    return;
  }
  const similarity = Number(data.similarity || 0);
  $('#duplicateInfo').innerHTML = `
    <div class="duplicate-name">${esc(duplicateStudent.name)}</div>
    <div>学号：${esc(duplicateStudent.student_no)}</div>
    <div>班级：${esc(duplicateStudent.class_name || '-')}</div>
    <div>现有人脸样本：${duplicateStudent.face_count || 1} 个</div>
    <div>本次相似度：${(similarity * 100).toFixed(1)}%</div>
  `;
  openModal('duplicateModal');
}

$('#duplicateViewSamples').addEventListener('click', async () => {
  if (!duplicateStudent) return;
  closeModal('duplicateModal');
  await openSamplesPanel(duplicateStudent.id);
});

function setEnrollmentStatus(title, detail, tone = 'neutral') {
  const box = $('#enrollStatus');
  if (!box) return;
  box.className = `capture-status ${tone}`;
  box.innerHTML = `<strong>${esc(title)}</strong><span>${esc(detail)}</span>`;
}

function setEnrollmentAngle(angle) {
  const label = $('#enrollAngleState');
  if (label) label.textContent = `当前角度：${angle || '正面'}`;
}

async function captureEnrollment() {
  try {
    await startCamera();
    setEnrollmentStatus('正在检测人脸', '请保持不动，系统正在检查人脸质量和重复记录。', 'working');
    const blob = await capture('#enrollCamera');

    if (supplementStudent) {
      const fd = new FormData();
      fd.append('file', blob, 'face.jpg');
      fd.append('label', $('#sampleAngle').value);
      try {
        await api(`/api/students/${supplementStudent.id}/faces`, {method: 'POST', body: fd});
      } catch (e) {
        if (e.data && e.data.duplicate) {
          setEnrollmentStatus('检测到重复人脸', '当前面孔已经属于其他已录入学生，请核对后再操作。', 'warning');
          showDuplicate(e.data);
          return;
        }
        throw e;
      }
      const studentID = supplementStudent.id;
      const angle = $('#sampleAngle').value;
      setEnrollmentStatus('样本录入成功', `${supplementStudent.name} 的“${angle}”样本已经保存。`, 'success');
      toast('多角度人脸样本已补充');
      cancelSupplementMode(false);
      await loadStudents();
      await openSamplesPanel(studentID, {scroll: false});
      return;
    }

    const checkForm = new FormData();
    checkForm.append('file', blob, 'face.jpg');
    const check = await api('/api/enrollment/check', {method: 'POST', body: checkForm});
    if (check.duplicate) {
      setEnrollmentStatus('该人脸已经录入', '系统已找到匹配学生，可在右侧查看已有样本。', 'warning');
      showDuplicate(check);
      return;
    }

    await loadClasses();
    if (!classesCache.length) {
      pendingEnrollmentBlob = null;
      toast('请先到设置 → 班级编排中添加班级');
      setEnrollmentStatus('需要先设置班级', '请先到设置中建立班级，再回来完成人脸录入。', 'warning');
      await showPage('settings');
      return;
    }

    setEnrollmentStatus('新面孔检测通过', '请在弹出的学生资料窗口中填写学号、姓名和班级。', 'success');
    pendingEnrollmentBlob = blob;
    $('#studentInfoForm').reset();
    renderClassOptions();
    openModal('studentModal');
    setTimeout(() => $('#modalStudentNo').focus(), 50);
  } catch (e) {
    setEnrollmentStatus('采集失败', e.message, 'error');
    toast(e.message);
  }
}

$('#toggleEnrollCamera').addEventListener('click', toggleCamera);
$('#captureEnrollment').addEventListener('click', captureEnrollment);

$('#studentInfoForm').addEventListener('submit', async e => {
  e.preventDefault();
  if (!pendingEnrollmentBlob) {
    toast('请重新拍摄人脸');
    closeModal('studentModal');
    return;
  }

  const fd = new FormData();
  fd.append('file', pendingEnrollmentBlob, 'face.jpg');
  fd.append('student_no', $('#modalStudentNo').value);
  fd.append('name', $('#modalStudentName').value);
  fd.append('class_name', $('#modalClassName').value);
  fd.append('label', '正面');

  try {
    await api('/api/enrollment/create', {method: 'POST', body: fd});
    pendingEnrollmentBlob = null;
    closeModal('studentModal');
    setEnrollmentStatus('首次录入完成', '学生资料与正面人脸样本已经保存，可继续补充其他角度。', 'success');
    toast('学生信息和人脸已录入');
    await loadStudents();
    await loadHealth();
  } catch (e2) {
    if (e2.data && e2.data.duplicate) {
      closeModal('studentModal');
      pendingEnrollmentBlob = null;
      showDuplicate(e2.data);
      return;
    }
    toast(e2.message);
  }
});
