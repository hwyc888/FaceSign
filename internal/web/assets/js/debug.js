const recognitionDebugFields = [
  ['debugMatchThreshold', 'match_threshold'],
  ['debugDetectionThreshold', 'detection_threshold'],
  ['debugLivenessPassThreshold', 'liveness_pass_threshold'],
  ['debugLivenessFrameThreshold', 'liveness_frame_threshold'],
  ['debugLivenessSpoofThreshold', 'liveness_spoof_threshold']
];

function renderRecognitionDebug(payload) {
  recognitionDebugFields.forEach(([id, key]) => {
    $('#' + id).value = Number(payload.current[key]).toFixed(2);
    $('#' + id + 'Default').textContent = Number(payload.defaults[key]).toFixed(2);
  });
  $('#debugRequiredFrames').textContent = payload.required_frames + ' 帧（固定）';
  $('#debugThresholdState').textContent = payload.modified ? '自定义配置' : '当前默认';
  $('#recognitionDebugMessage').textContent = payload.modified
    ? '当前使用自定义阈值。点击“恢复默认配置”可回到本次启动时的原始参数。'
    : '当前使用本版本启动时的原始识别阈值。';
}

async function loadRecognitionDebugThresholds() {
  if (!currentUser || !currentUser.is_admin) return;
  try {
    renderRecognitionDebug(await api('/api/debug/thresholds'));
  } catch (error) {
    $('#debugThresholdState').textContent = '读取失败';
    $('#recognitionDebugMessage').textContent = error.message;
  }
}

const recognitionDebugForm = $('#recognitionDebugForm');
recognitionDebugForm.addEventListener('submit', async event => {
  event.preventDefault();
  const value = {};
  for (const [id, key] of recognitionDebugFields) {
    const raw = $('#' + id).value.trim();
    const num = Number(raw);
    if (raw === '' || !Number.isFinite(num)) {
      toast('请填写有效的数值');
      return;
    }
    value[key] = num;
  }
  const save = $('#saveRecognitionDebug');
  const reset = $('#resetRecognitionDebug');
  save.disabled = reset.disabled = true;
  try {
    const result = await api('/api/debug/thresholds', {
      method: 'PUT',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify(value)
    });
    renderRecognitionDebug(result);
    toast('识别参数已保存并立即生效');
    loadHealth();
  } catch (error) {
    $('#recognitionDebugMessage').textContent = error.message;
    toast(error.message);
  } finally {
    save.disabled = reset.disabled = false;
  }
});

$('#resetRecognitionDebug').addEventListener('click', async () => {
  if (!(await showConfirmDialog('确认恢复本次启动的默认识别阈值吗？\n此操作不会清除学生、样本、考勤或其他设置。', {title: '恢复默认阈值', confirmText: '恢复默认'}))) return;
  const save = $('#saveRecognitionDebug');
  const reset = $('#resetRecognitionDebug');
  save.disabled = reset.disabled = true;
  try {
    const result = await api('/api/debug/thresholds/reset', {method: 'POST'});
    renderRecognitionDebug(result);
    toast('已恢复本次启动的默认识别阈值');
    loadHealth();
  } catch (error) {
    $('#recognitionDebugMessage').textContent = error.message;
    toast(error.message);
  } finally {
    save.disabled = reset.disabled = false;
  }
});
