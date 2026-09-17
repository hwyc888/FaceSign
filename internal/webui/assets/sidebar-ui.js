// Keep the new left navigation in sync with the existing FaceSign view state.
(() => {
  const adminMeta = {
    teachers: ['教师管理', '维护可登录教师账号，授课教师将在课程管理中直接选择。'],
    classes: ['班级管理', '建立班级基础资料，学生录入时直接归属班级。'],
    students: ['学生与人脸录入', '集中维护学生资料，并完成刷脸考勤所需的人脸模板录入。'],
    courses: ['课程管理', '维护课程与授课教师关系，为课表和考勤场次提供基础数据。'],
    enrollment: ['课程学生名单', '把学生加入具体课程，确保刷脸签到只匹配本课程名单。'],
    schedules: ['课表管理', '按课程时间维护课表，系统据此自动开启和结束考勤场次。'],
    face: ['人脸识别服务', '查看本地 CPU 人脸引擎状态，并管理识别参数和服务地址。']
  };

  function syncNavigation() {
    if (typeof state === 'undefined') return;

    document.querySelectorAll('.topbar .nav .active').forEach(node => node.classList.remove('active'));
    if (state.view === 'dashboard') {
      document.querySelector('.topbar [data-view="dashboard"]')?.classList.add('active');
    } else if (state.view === 'admin' && state.adminSection) {
      document.querySelector(`.topbar [data-admin-nav="${state.adminSection}"]`)?.classList.add('active');
    }

    if (state.view !== 'admin' || !state.adminSection) return;
    const meta = adminMeta[state.adminSection];
    const header = document.querySelector('.admin-header .section-title > div');
    if (!meta || !header) return;
    const title = header.querySelector('h2');
    const description = header.querySelector('.muted');
    if (title) title.textContent = meta[0];
    if (description) description.textContent = meta[1];
  }

  const observer = new MutationObserver(syncNavigation);
  observer.observe(app, { childList: true, subtree: true });
  document.addEventListener('click', event => {
    if (event.target.closest('.topbar [data-view], .topbar [data-admin-nav]')) {
      setTimeout(syncNavigation, 0);
    }
  }, true);
  syncNavigation();
})();
