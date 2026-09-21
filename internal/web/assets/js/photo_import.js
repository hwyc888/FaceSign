let photoImportSessionID = '';
let photoImportItems = [];

function photoImportStatusMeta(item) {
  switch (item.status) {
    case 'ready': return {label: '可导入', tone: 'good'};
    case 'existing': return {label: '已匹配现有学生', tone: 'warn'};
    case 'review': return {label: '需要人工复核', tone: 'warn'};
    case 'package_duplicate': return {label: '压缩包内疑似重复', tone: 'warn'};
    default: return {label: '不合格', tone: 'bad'};
  }
}

function photoImportActionOptions(item) {
  if (item.status === 'ready') {
    return '<option value="new" selected>新建学生 + 正面样本</option><option value="skip">跳过</option>';
  }
  if (item.status === 'existing') {
    return '<option value="skip" selected>跳过（已有）</option><option value="supplement">补充现有学生正面样本</option>';
  }
  return '<option value="skip" selected>跳过</option>';
}

function renderPhotoImportClassOptions() {
  const bulk = $('#photoImportBulkClass');
  if (bulk) bulk.innerHTML = classOptionsHTML('', true);
}

function photoImportClassSelect(item) {
  return '<select data-import-class="' + item.id + '">' + classOptionsHTML(item.class_name || '', true) + '</select>';
}

function importSelector(name, id) {
  return '[' + name + '="' + id + '"]';
}

function renderPhotoImportPreview(data) {
  photoImportSessionID = data.session_id;
  photoImportItems = data.items || [];
  renderPhotoImportClassOptions();

  const counts = data.counts || {};
  $('#photoImportSummary').innerHTML = [
    '<span class="import-summary-chip">共分析 ' + photoImportItems.length + ' 张</span>',
    '<span class="import-summary-chip good">可新建 ' + (counts.ready || 0) + '</span>',
    '<span class="import-summary-chip warn">已有 ' + (counts.existing || 0) + '</span>',
    '<span class="import-summary-chip warn">需复核 ' + (counts.review || 0) + '</span>',
    '<span class="import-summary-chip bad">不合格/重复 ' + ((counts.rejected || 0) + (counts.package_duplicate || 0)) + '</span>',
    data.ignored_files ? '<span class="import-summary-chip">忽略非图片 ' + data.ignored_files + '</span>' : ''
  ].join('');

  $('#photoImportBody').innerHTML = photoImportItems.map(function(item) {
    const meta = photoImportStatusMeta(item);
    const matched = item.existing
      ? '<span>匹配：' + esc(item.existing.name) + ' · ' + (Number(item.similarity || 0) * 100).toFixed(1) + '%</span>'
      : '';
    const duplicate = item.duplicate_file ? '<span>疑似同人：' + esc(item.duplicate_file) + '</span>' : '';
    const reason = item.reason
      ? '<span>' + esc(item.reason) + '</span>'
      : '<span>' + esc(item.quality_note || '') + '</span>';
    const editable = item.status === 'ready';
    const thumb = item.thumbnail_url
      ? '<img class="photo-thumb" src="' + esc(item.thumbnail_url) + '" alt="">'
      : '<div class="photo-thumb"></div>';
    return '<tr data-import-row="' + item.id + '">' +
      '<td>' + thumb + '</td>' +
      '<td><div class="photo-import-file-name">' + esc(item.file_name) + '</div><span class="badge ' + (item.quality === '优' ? 'yes' : 'no') + '">' + esc(item.quality || '-') + '</span></td>' +
      '<td><input data-import-no="' + item.id + '" value="' + esc(item.student_no || '') + '" ' + (editable ? '' : 'disabled') + ' placeholder="学号"></td>' +
      '<td><input data-import-name="' + item.id + '" value="' + esc(item.name || '') + '" ' + (editable ? '' : 'disabled') + ' placeholder="姓名"></td>' +
      '<td>' + photoImportClassSelect(item) + '</td>' +
      '<td><div class="import-status ' + meta.tone + '"><strong>' + meta.label + '</strong>' + matched + duplicate + reason + '</div></td>' +
      '<td><select data-import-action="' + item.id + '">' + photoImportActionOptions(item) + '</select></td>' +
      '</tr>';
  }).join('');

  $$('[data-import-action]').forEach(function(select) {
    select.onchange = function() {
      updatePhotoImportRowState(Number(select.dataset.importAction));
    };
  });
  photoImportItems.forEach(function(item) {
    updatePhotoImportRowState(item.id);
  });
  $('#photoImportPreview').classList.remove('hidden');
}

function updatePhotoImportRowState(id) {
  const actionElement = $(importSelector('data-import-action', id));
  const action = actionElement ? actionElement.value : 'skip';
  const item = photoImportItems.find(function(row) { return Number(row.id) === Number(id); });
  if (!item) return;
  const isNew = action === 'new';
  const no = $(importSelector('data-import-no', id));
  const name = $(importSelector('data-import-name', id));
  const classSelect = $(importSelector('data-import-class', id));
  if (no) no.disabled = !isNew;
  if (name) name.disabled = !isNew;
  if (classSelect) classSelect.disabled = action === 'skip';
}

$('#analyzePhotoImport').addEventListener('click', async function() {
  const input = $('#photoImportFile');
  if (!input.files || !input.files[0]) {
    toast('请选择 ZIP 照片压缩包');
    return;
  }
  const button = $('#analyzePhotoImport');
  button.disabled = true;
  $('#photoImportProgress').classList.remove('hidden');
  $('#photoImportPreview').classList.add('hidden');
  try {
    await loadClasses();
    const fd = new FormData();
    fd.append('file', input.files[0], input.files[0].name);
    const data = await api('/api/photo-import/analyze', {method: 'POST', body: fd});
    renderPhotoImportPreview(data);
    toast('照片分析完成，请检查后确认导入');
  } catch (e) {
    toast(e.message);
  } finally {
    button.disabled = false;
    $('#photoImportProgress').classList.add('hidden');
  }
});

$('#applyPhotoImportClass').addEventListener('click', function() {
  const className = $('#photoImportBulkClass').value;
  if (!className) {
    toast('请先选择班级');
    return;
  }
  photoImportItems.forEach(function(item) {
    const actionElement = $(importSelector('data-import-action', item.id));
    const classSelect = $(importSelector('data-import-class', item.id));
    const action = actionElement ? actionElement.value : 'skip';
    if (classSelect && action !== 'skip') classSelect.value = className;
  });
  toast('班级已批量应用');
});

$('#commitPhotoImport').addEventListener('click', async function() {
  if (!photoImportSessionID) {
    toast('请先上传并分析照片');
    return;
  }
  const items = photoImportItems.map(function(item) {
    const actionElement = $(importSelector('data-import-action', item.id));
    const noElement = $(importSelector('data-import-no', item.id));
    const nameElement = $(importSelector('data-import-name', item.id));
    const classElement = $(importSelector('data-import-class', item.id));
    return {
      id: item.id,
      action: actionElement ? actionElement.value : 'skip',
      student_no: noElement ? noElement.value : (item.student_no || ''),
      name: nameElement ? nameElement.value : (item.name || ''),
      class_name: classElement ? classElement.value : ''
    };
  });
  const selected = items.filter(function(item) { return item.action !== 'skip'; });
  if (!selected.length) {
    toast('没有选择需要导入的照片');
    return;
  }
  if (selected.some(function(item) { return !item.class_name; })) {
    toast('所有要导入的照片都必须选择班级');
    return;
  }
  if (selected.some(function(item) {
    return item.action === 'new' && (!item.student_no.trim() || !item.name.trim());
  })) {
    toast('新建学生必须填写学号和姓名');
    return;
  }

  const button = $('#commitPhotoImport');
  button.disabled = true;
  try {
    const result = await api('/api/photo-import/' + photoImportSessionID + '/commit', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({items: items})
    });
    toast('导入完成：成功 ' + result.imported + '，跳过 ' + result.skipped + '，失败 ' + result.failed);
    await loadStudents();
    await loadHealth();
    (result.results || []).forEach(function(row) {
      if (row.status === 'imported' || row.status === 'skipped') {
        const action = $(importSelector('data-import-action', row.id));
        if (action) {
          action.innerHTML = '<option value="skip">已处理</option>';
          action.disabled = true;
        }
      }
    });
  } catch (e) {
    toast(e.message);
  } finally {
    button.disabled = false;
  }
});
