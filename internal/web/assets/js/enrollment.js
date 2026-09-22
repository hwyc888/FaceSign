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
