let usersCache = [];

async function loadUsers() {
  if (!currentUser || !currentUser.is_admin) return;
  try {
    usersCache = await api('/api/users');
    const canManageAdminRoles = Boolean(currentUser.is_primary_admin);
    $('#usersBody').innerHTML = usersCache.map(user => {
      const checkmark = '<td>✓</td>';
      if (user.is_primary_admin) {
        return '<tr><td><strong>' + esc(user.username) + '</strong><br><span class="user-account-admin">首个管理员 · 最高权限 · 不可撤销</span></td>' +
          checkmark.repeat(5) + '<td>固定管理员</td></tr>';
      }
      if (user.is_admin) {
        const roleAction = canManageAdminRoles
          ? '<button class="danger" data-user-revoke-admin="' + user.id + '">撤销管理员</button>'
          : '<span class="muted compact">仅首个管理员可撤销</span>';
        return '<tr data-user-row="' + user.id + '">' +
          '<td><strong>' + esc(user.username) + '</strong><br><span class="user-account-admin">授权管理员 · 全部功能已开放</span></td>' +
          checkmark.repeat(5) +
          '<td><div class="user-row-actions">' + roleAction + '</div></td></tr>';
      }
      const permissionCell = (key, checked) =>
        '<td><label><input type="checkbox" data-user-permission="' + key +
        '" data-user-id="' + user.id + '"' + (checked ? ' checked' : '') + '>允许</label></td>';
      const grantAction = canManageAdminRoles
        ? '<button class="primary-soft" data-user-grant-admin="' + user.id + '">授权为管理员</button>'
        : '';
      return '<tr data-user-row="' + user.id + '">' +
        '<td><strong>' + esc(user.username) + '</strong><br><span class="muted compact">普通用户</span></td>' +
        permissionCell('can_edit_students', user.can_edit_students) +
        permissionCell('can_add_students', user.can_add_students) +
        permissionCell('can_resample_faces', user.can_resample_faces) +
        permissionCell('can_arrange_seats', user.can_arrange_seats) +
        permissionCell('can_edit_settings', user.can_edit_settings) +
        '<td><div class="user-row-actions"><button data-user-clone="' + user.id + '">复制账号</button>' +
        '<button class="primary-soft" data-user-save="' + user.id + '">保存权限</button>' +
        '<button class="danger" data-user-delete="' + user.id + '">删除账号</button>' +
        grantAction + '</div></td></tr>';
    }).join('') || '<tr><td colspan="7">暂无用户</td></tr>';

    $$('[data-user-clone]').forEach(button => {
      button.onclick = () => openUserCloneModal(Number(button.dataset.userClone), 'single');
    });

    $$('[data-user-save]').forEach(button => {
      button.onclick = async () => {
        const userID = Number(button.dataset.userSave);
        const values = {};
        $$('[data-user-permission]').forEach(input => {
          if (Number(input.dataset.userId) === userID) {
            values[input.dataset.userPermission] = input.checked;
          }
        });
        button.disabled = true;
        try {
          await api('/api/users/' + userID, {
            method: 'PUT',
            headers: {'Content-Type': 'application/json'},
            body: JSON.stringify(values)
          });
          toast('用户权限已保存');
          await loadUsers();
        } catch (error) {
          toast(error.message);
        } finally {
          button.disabled = false;
        }
      };
    });

    $$('[data-user-delete]').forEach(button => {
      button.onclick = async () => {
        const userID = Number(button.dataset.userDelete);
        const user = usersCache.find(item => Number(item.id) === userID);
        if (!user || !(await showConfirmDialog('确定删除普通用户“' + user.username + '”？该账号将立即无法登录。', {title: '删除用户', confirmText: '删除用户', confirmClass: 'danger'}))) return;
        button.disabled = true;
        try {
          await api('/api/users/' + userID, {method: 'DELETE'});
          toast('用户账号已删除');
          await loadUsers();
        } catch (error) {
          toast(error.message);
          button.disabled = false;
        }
      };
    });

    $$('[data-user-grant-admin]').forEach(button => {
      button.onclick = async () => {
        const userID = Number(button.dataset.userGrantAdmin);
        const user = usersCache.find(item => Number(item.id) === userID);
        if (!user || !canManageAdminRoles) return;
        if (!(await showConfirmDialog('确定将“' + user.username + '”授权为管理员？授权后将解锁全部系统页面、识别阈值、活体参数、摄像头和人员管理功能。该账号需要重新登录后生效。', {title: '授权管理员', confirmText: '确认授权'}))) return;
        button.disabled = true;
        try {
          await api('/api/users/' + userID + '/admin', {method: 'POST'});
          toast('已授权为管理员；该账号重新登录后即可使用全部管理功能');
          await loadUsers();
        } catch (error) {
          toast(error.message);
          button.disabled = false;
        }
      };
    });

    $$('[data-user-revoke-admin]').forEach(button => {
      button.onclick = async () => {
        const userID = Number(button.dataset.userRevokeAdmin);
        const user = usersCache.find(item => Number(item.id) === userID);
        if (!user || !user.is_admin || user.is_primary_admin || !canManageAdminRoles) return;
        if (!(await showConfirmDialog('确定撤销“' + user.username + '”的管理员身份？该账号将恢复授权前的普通权限，并需要重新登录。', {title: '撤销管理员', confirmText: '撤销权限', confirmClass: 'danger'}))) return;
        button.disabled = true;
        try {
          await api('/api/users/' + userID + '/admin', {method: 'DELETE'});
          toast('已撤销管理员身份；原有普通权限已保留');
          await loadUsers();
        } catch (error) {
          toast(error.message);
          button.disabled = false;
        }
      };
    });
  } catch (error) {
    toast(error.message);
  }
}

$('#userCreateForm').addEventListener('submit', async event => {
  event.preventDefault();
  const button = $('#userCreateForm').querySelector('button[type="submit"]');
  button.disabled = true;
  try {
    await api('/api/users', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({
        username: $('#newUserUsername').value.trim(),
        password: $('#newUserPassword').value,
        can_edit_students: $('#newUserCanEdit').checked,
        can_add_students: $('#newUserCanAdd').checked,
        can_resample_faces: $('#newUserCanResample').checked,
        can_arrange_seats: $('#newUserCanArrange').checked,
        can_edit_settings: $('#newUserCanEditSettings').checked
      })
    });
    $('#userCreateForm').reset();
    toast('普通用户账号已创建');
    await loadUsers();
  } catch (error) {
    toast(error.message);
  } finally {
    button.disabled = false;
  }
});

$('#refreshUsers').addEventListener('click', loadUsers);


function ordinaryUsersForClone() {
  return usersCache.filter(user => !user.is_admin);
}

function openUserCloneModal(sourceID = 0, mode = 'single') {
  const sources = ordinaryUsersForClone();
  if (!sources.length) {
    toast('目前没有可复制的普通用户，请先创建一个普通用户');
    return;
  }
  $('#userCloneForm').reset();
  $('#userCloneSource').innerHTML = '<option value="">请选择普通用户</option>' +
    sources.map(user => '<option value="' + user.id + '">' + esc(user.username) + '</option>').join('');
  const selectedID = Number(sourceID) || Number(sources[0].id);
  $('#userCloneSource').value = String(selectedID);
  $('#userCloneMode').value = mode;
  updateUserCloneMode();
  renderUserClonePermissionPreview();
  renderUserClonePreview();
  openModal('userCloneModal');
}

function updateUserCloneMode() {
  const batch = $('#userCloneMode').value === 'batch';
  $('#userCloneSingleFields').classList.toggle('hidden', batch);
  $('#userCloneBatchFields').classList.toggle('hidden', !batch);
  $('#userCloneSubmit').textContent = batch ? '批量创建账号' : '创建复制账号';
  renderUserClonePreview();
}

function buildUserCloneNames() {
  if ($('#userCloneMode').value !== 'batch') {
    const username = $('#userCloneUsername').value.trim();
    if (username.length < 3 || username.length > 32) {
      throw new Error('新用户名长度必须为 3 到 32 个字符');
    }
    return [username];
  }

  const prefix = $('#userClonePrefix').value.trim();
  const start = Number($('#userCloneStart').value);
  const count = Number($('#userCloneCount').value);
  const digits = Number($('#userCloneDigits').value);
  if (!prefix) throw new Error('请输入用户名固定前缀');
  if (!Number.isInteger(start) || start < 0 || start > 99999999) {
    throw new Error('起始编号必须在 0 到 99999999 之间');
  }
  if (!Number.isInteger(count) || count < 1 || count > 100) {
    throw new Error('批量生成数量必须在 1 到 100 之间');
  }
  if (!Number.isInteger(digits) || digits < 1 || digits > 8) {
    throw new Error('编号位数必须在 1 到 8 之间');
  }
  return Array.from({length: count}, (_, index) =>
    prefix + String(start + index).padStart(digits, '0')
  );
}

function findUserCloneCollision(usernames) {
  const existing = new Set(usersCache.map(user => user.username.toLowerCase()));
  const seen = new Set();
  for (const username of usernames) {
    const key = username.toLowerCase();
    if (seen.has(key)) return '生成的用户名重复：' + username;
    if (existing.has(key)) return '用户名已存在：' + username;
    seen.add(key);
  }
  return '';
}

function renderUserClonePreview() {
  const preview = $('#userClonePreview');
  const hint = $('#userClonePreviewHint');
  if (!preview || !hint) return;
  try {
    const usernames = buildUserCloneNames();
    preview.textContent = usernames.slice(0, 8).join('、') +
      (usernames.length > 8 ? ' ……（共 ' + usernames.length + ' 个）' : '');
    const collision = findUserCloneCollision(usernames);
    hint.textContent = collision || ('将创建 ' + usernames.length + ' 个普通账号，权限继承所选来源。');
    hint.classList.toggle('error', Boolean(collision));
  } catch (error) {
    preview.textContent = '等待输入生成规则';
    hint.textContent = error.message;
    hint.classList.add('error');
  }
}

function renderUserClonePermissionPreview() {
  const sourceID = Number($('#userCloneSource').value);
  const source = usersCache.find(user => Number(user.id) === sourceID && !user.is_admin);
  const preview = $('#userClonePermissionPreview');
  if (!source) {
    preview.textContent = '请选择一个普通用户作为复制来源。';
    return;
  }
  const permissions = [
    ['修改学生资料', source.can_edit_students],
    ['新增学生', source.can_add_students],
    ['补充 / 重新采样人脸', source.can_resample_faces],
    ['座位编排', source.can_arrange_seats],
    ['修改设置', source.can_edit_settings]
  ];
  preview.innerHTML = permissions.map(([label, allowed]) =>
    '<span class="user-clone-permission ' + (allowed ? 'allowed' : 'denied') + '">' +
    label + '：' + (allowed ? '允许' : '未授权') + '</span>'
  ).join('');
}

$('#userCloneMode').addEventListener('change', updateUserCloneMode);
$('#userCloneSource').addEventListener('change', renderUserClonePermissionPreview);
['userCloneUsername', 'userClonePrefix', 'userCloneStart', 'userCloneCount', 'userCloneDigits'].forEach(id => {
  $('#' + id).addEventListener('input', renderUserClonePreview);
});

$('#openUserCloneBatch').addEventListener('click', () => {
  openUserCloneModal(0, 'batch');
});

$('#userCloneForm').addEventListener('submit', async event => {
  event.preventDefault();
  const sourceID = Number($('#userCloneSource').value);
  if (!sourceID) {
    toast('请选择复制来源');
    return;
  }
  const password = $('#userClonePassword').value;
  if (password !== $('#userClonePasswordConfirm').value) {
    toast('两次输入的统一密码不一致');
    return;
  }

  let usernames;
  try {
    usernames = buildUserCloneNames();
    const collision = findUserCloneCollision(usernames);
    if (collision) throw new Error(collision);
  } catch (error) {
    toast(error.message);
    return;
  }

  const button = $('#userCloneSubmit');
  button.disabled = true;
  try {
    const result = await api('/api/users/' + sourceID + '/clone', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({usernames, password})
    });
    closeModal('userCloneModal');
    $('#userCloneForm').reset();
    toast('已复制 ' + result.created + ' 个普通账号，权限已继承，统一密码已设置');
    await loadUsers();
  } catch (error) {
    toast(error.message);
  } finally {
    button.disabled = false;
  }
});

