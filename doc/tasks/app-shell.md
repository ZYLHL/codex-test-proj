# app-shell 模块最小任务

- [ ] 搭建 Vue 应用入口与基础布局（Header / NotesWall / Footer 占位）。
- [ ] 建立全局应用状态：`locked | unlocked`、`online | offline`、`syncStatus`。
- [ ] 实现解锁前路由守卫：未解锁仅允许进入解锁页。
- [ ] 监听浏览器网络状态变更并发布事件给同步模块。
- [ ] 增加最小集成验证：切换锁定状态时页面权限正确。
