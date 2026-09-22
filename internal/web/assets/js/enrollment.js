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

let pendingDuplicateProfileUpdate = null;

function duplicateProfileClassOptions(selected) {
  return classesCache.map(item =>
    '<option value="' + esc(item.name) + '" ' + (item.name === selected ? 'selected' : '') + '>' + esc(item.name) + '</option>'
  ).join('');
}

function duplicateProfileChanges(next) {
  if (!duplicateStudent) return [];
  const fields = [
    ['学号', duplicateStudent.student_no || '', next.student_no],
    ['姓名', duplicateStudent.name || '', next.name],
    ['班级', duplicateStudent.class_name || '', next.class_name]
  ];
  const changes = fields
    .filter(([, oldValue, newValue]) => String(oldValue).trim() !== String(newValue).trim())
    .map(([label, oldValue, newValue]) => ({label, oldValue: String(oldValue || '-'), newValue: String(newValue || '-')}));
  if (
    String(duplicateStudent.class_name || '').trim() !== String(next.class_name || '').trim() &&
    Number(duplicateStudent.seat_no || 0) > 0
  ) {
    changes.push({
      label: '座位号',
      oldValue: String(duplicateStudent.seat_no),
      newValue: '未编排（跨班自动清空）',
      consequence: true
    });
  }
  return changes;
}

$('#duplicateUpdateProfile').addEventListener('click', async () => {
  if (!duplicateStudent) return;
  try {
    await loadClasses();
    $('#duplicateUpdateStudentNo').value = duplicateStudent.student_no || '';
    $('#duplicateUpdateName').value = duplicateStudent.name || '';
    $('#duplicateUpdateClassName').innerHTML = duplicateProfileClassOptions(duplicateStudent.class_name || '');
    pendingDuplicateProfileUpdate = null;
    closeModal('duplicateModal');
    openModal('duplicateUpdateModal');
    setTimeout(() => $('#duplicateUpdateStudentNo').focus(), 50);
  } catch (e) {
    toast(e.message);
  }
});

$('#duplicateUpdateForm').addEventListener('submit', e => {
  e.preventDefault();
  if (!duplicateStudent) return;
  const next = {
    student_no: $('#duplicateUpdateStudentNo').value.trim(),
    name: $('#duplicateUpdateName').value.trim(),
    class_name: $('#duplicateUpdateClassName').value.trim()
  };
  if (!next.student_no || !next.name || !next.class_name) {
    toast('学号、姓名和班级都不能为空');
    return;
  }

  const changes = duplicateProfileChanges(next);
  if (!changes.length) {
    pendingDuplicateProfileUpdate = null;
    setEnrollmentStatus('资料无需更新', '本次填写内容与系统中原资料完全一致。', 'neutral');
    toast('资料与原记录一致，无需更新');
    closeModal('duplicateUpdateModal');
    return;
  }

  pendingDuplicateProfileUpdate = {...next, changes};
  $('#duplicateDiffList').innerHTML = changes.map(change => `
    <div class="profile-diff-row ${change.consequence ? 'consequence' : ''}">
      <strong>${esc(change.label)}</strong>
      <span class="profile-diff-old">${esc(change.oldValue)}</span>
      <span class="profile-diff-arrow">→</span>
      <span class="profile-diff-new">${esc(change.newValue)}</span>
    </div>
  `).join('');
  const classChanged = String(duplicateStudent.class_name || '').trim() !== next.class_name;
  const warning = $('#duplicateDiffWarning');
  warning.classList.toggle('hidden', !classChanged);
  warning.textContent = classChanged ? '班级发生变化时，原座位号会自动清空，需要到班级编排中重新安排座位。' : '';
  closeModal('duplicateUpdateModal');
  openModal('duplicateDiffModal');
});

$('#duplicateBackToEdit').addEventListener('click', () => {
  closeModal('duplicateDiffModal');
  openModal('duplicateUpdateModal');
});

$('#duplicateKeepOriginal').addEventListener('click', () => {
  pendingDuplicateProfileUpdate = null;
  closeModal('duplicateDiffModal');
  setEnrollmentStatus('资料保持不变', '已选择不更新，系统继续保留原学生资料。', 'neutral');
  toast('已保留原学生资料');
});

$('#duplicateConfirmUpdate').addEventListener('click', async () => {
  if (!duplicateStudent || !pendingDuplicateProfileUpdate) return;
  const button = $('#duplicateConfirmUpdate');
  button.disabled = true;
  try {
    const result = await api('/api/students/' + duplicateStudent.id, {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        student_no: pendingDuplicateProfileUpdate.student_no,
        name: pendingDuplicateProfileUpdate.name,
        class_name: pendingDuplicateProfileUpdate.class_name
      })
    });
    const updated = result.student || {
      ...duplicateStudent,
      student_no: pendingDuplicateProfileUpdate.student_no,
      name: pendingDuplicateProfileUpdate.name,
      class_name: pendingDuplicateProfileUpdate.class_name,
      seat_no: duplicateStudent.class_name === pendingDuplicateProfileUpdate.class_name ? duplicateStudent.seat_no : 0
    };
    const changedCount = pendingDuplicateProfileUpdate.changes.filter(change => !change.consequence).length;
    duplicateStudent = updated;
    pendingDuplicateProfileUpdate = null;
    closeModal('duplicateDiffModal');
    setEnrollmentStatus('学生资料已更新', '已确认并保存 ' + changedCount + ' 项资料变更。', 'success');
    toast('学生资料已更新');
    await loadStudents();
    if (samplesStudent && Number(samplesStudent.id) === Number(updated.id)) {
      samplesStudent = updated;
      $('#sampleStudentName').textContent = updated.name;
      $('#sampleStudentNo').textContent = updated.student_no;
      $('#sampleStudentClass').textContent = updated.class_name || '-';
    }
  } catch (e) {
    toast(e.message);
  } finally {
    button.disabled = false;
  }
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


const enrollmentFaceGuidePosition = {x: 50, y: 50};

function enrollmentFaceGuideBounds() {
  const guide = $('#enrollFaceGuide');
  const wrap = guide?.closest('.enroll-video-wrap');
  if (!guide || !wrap) return null;
  const wrapRect = wrap.getBoundingClientRect();
  if (!wrapRect.width || !wrapRect.height) return null;
  const guideRect = guide.getBoundingClientRect();
  return {
    minX: (guideRect.width / wrapRect.width) * 50,
    maxX: 100 - (guideRect.width / wrapRect.width) * 50,
    minY: (guideRect.height / wrapRect.height) * 50,
    maxY: 100 - (guideRect.height / wrapRect.height) * 50,
    width: wrapRect.width,
    height: wrapRect.height
  };
}

function setEnrollmentFaceGuidePosition(x, y) {
  const guide = $('#enrollFaceGuide');
  const bounds = enrollmentFaceGuideBounds();
  if (!guide || !bounds) return;
  enrollmentFaceGuidePosition.x = Math.max(bounds.minX, Math.min(bounds.maxX, Number(x) || 50));
  enrollmentFaceGuidePosition.y = Math.max(bounds.minY, Math.min(bounds.maxY, Number(y) || 50));
  guide.style.left = enrollmentFaceGuidePosition.x + '%';
  guide.style.top = enrollmentFaceGuidePosition.y + '%';
}

function resetEnrollmentFaceGuide(showToast = false) {
  setEnrollmentFaceGuidePosition(50, 50);
  if (showToast) toast('人脸引导框已恢复居中');
}

function initializeEnrollmentFaceGuide() {
  const guide = $('#enrollFaceGuide');
  if (!guide || guide.dataset.dragReady === '1') return;
  guide.dataset.dragReady = '1';

  let drag = null;

  guide.addEventListener('pointerdown', event => {
    if (event.button !== undefined && event.button !== 0) return;
    const bounds = enrollmentFaceGuideBounds();
    if (!bounds) return;
    drag = {
      pointerId: event.pointerId,
      startClientX: event.clientX,
      startClientY: event.clientY,
      startX: enrollmentFaceGuidePosition.x,
      startY: enrollmentFaceGuidePosition.y,
      width: bounds.width,
      height: bounds.height
    };
    guide.classList.add('dragging');
    guide.focus({preventScroll: true});
    if (guide.setPointerCapture) guide.setPointerCapture(event.pointerId);
    event.preventDefault();
  });

  guide.addEventListener('pointermove', event => {
    if (!drag || event.pointerId !== drag.pointerId) return;
    const nextX = drag.startX + ((event.clientX - drag.startClientX) / drag.width) * 100;
    const nextY = drag.startY + ((event.clientY - drag.startClientY) / drag.height) * 100;
    setEnrollmentFaceGuidePosition(nextX, nextY);
    event.preventDefault();
  });

  const finishDrag = event => {
    if (!drag || (event.pointerId !== undefined && event.pointerId !== drag.pointerId)) return;
    if (guide.releasePointerCapture && guide.hasPointerCapture?.(drag.pointerId)) {
      guide.releasePointerCapture(drag.pointerId);
    }
    drag = null;
    guide.classList.remove('dragging');
  };

  guide.addEventListener('pointerup', finishDrag);
  guide.addEventListener('pointercancel', finishDrag);
  guide.addEventListener('lostpointercapture', () => {
    drag = null;
    guide.classList.remove('dragging');
  });

  guide.addEventListener('dblclick', event => {
    resetEnrollmentFaceGuide(true);
    event.preventDefault();
  });

  guide.addEventListener('keydown', event => {
    if (event.key === 'Home') {
      resetEnrollmentFaceGuide(true);
      event.preventDefault();
      return;
    }
    const step = event.shiftKey ? 4 : 1;
    let x = enrollmentFaceGuidePosition.x;
    let y = enrollmentFaceGuidePosition.y;
    if (event.key === 'ArrowLeft') x -= step;
    else if (event.key === 'ArrowRight') x += step;
    else if (event.key === 'ArrowUp') y -= step;
    else if (event.key === 'ArrowDown') y += step;
    else return;
    setEnrollmentFaceGuidePosition(x, y);
    event.preventDefault();
  });

  window.addEventListener('resize', () => {
    setEnrollmentFaceGuidePosition(enrollmentFaceGuidePosition.x, enrollmentFaceGuidePosition.y);
  });

  resetEnrollmentFaceGuide(false);
}

initializeEnrollmentFaceGuide();
