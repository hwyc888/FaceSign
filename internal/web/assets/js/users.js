let usersCache = [];

async function loadUsers() {
  if (!currentUser || !currentUser.is_admin) return;
  try {
    usersCache = await api('/api/users');
    $('#usersBody').innerHTML = usersCache.map(user => {
      if (user.is_admin) {
        return '<tr><td><strong>' + esc(user.username) + '</strong><br><span class="user-account-admin">系统管理员 · 不可删除</span></td>' +
          '<td>✓</td><td>✓</td><td>✓</td><td>✓</td><td>固定管理员</td></tr>';
      }
      const permissionCell = (key, checked) =>
        '<td><label><input type="checkbox" data-user-permission="' + key +
        '" data-user-id="' + user.id + '"' + (checked ? ' checked' : '') + '>允许</label></td>';
      return '<tr data-user-row="' + user.id + '">' +
        '<td><strong>' + esc(user.username) + '</strong><br><span class="muted compact">普通用户</span></td>' +
        permissionCell('can_edit_students', user.can_edit_students) +
        permissionCell('can_add_students', user.can_add_students) +
        permissionCell('can_resample_faces', user.can_resample_faces) +
        permissionCell('can_arrange_seats', user.can_arrange_seats) +
        '<td><div class="user-row-actions"><button class="primary-soft" data-user-save="' + user.id + '">保存权限</button>' +
        '<button class="danger" data-user-delete="' + user.id + '">删除账号</button></div></td></tr>';
    }).join('') || '<tr><td colspan="6">暂无用户</td></tr>';

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
        if (!user || !confirm('确定删除普通用户“' + user.username + '”？该账号将立即无法登录。')) return;
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
        can_arrange_seats: $('#newUserCanArrange').checked
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
