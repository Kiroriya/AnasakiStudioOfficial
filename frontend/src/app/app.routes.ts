import { Routes } from '@angular/router';
import { authGuard, capGuard, loginGuard, setupGuard } from './core/auth';
import { pluginPageGuard } from './core/plugins';
import { postTypeGuard } from './core/post-types';

export const routes: Routes = [
  { path: 'setup', canActivate: [setupGuard], loadComponent: () => import('./admin/setup').then(m => m.Setup) },
  { path: 'forgot', canActivate: [loginGuard], loadComponent: () => import('./admin/password-pages').then(m => m.ForgotPassword) },
  { path: 'reset', loadComponent: () => import('./admin/password-pages').then(m => m.ResetPassword) },
  { path: 'login', canActivate: [loginGuard], loadComponent: () => import('./admin/login').then(m => m.Login) },
  {
    path: 'design/component/:id', canActivate: [authGuard, capGuard('edit_pages')], data: { kind: 'component' },
    loadComponent: () => import('./editor/editor').then(m => m.Editor),
  },
  {
    path: 'design/:id', canActivate: [authGuard, capGuard('edit_pages')], data: { kind: 'post' },
    loadComponent: () => import('./editor/editor').then(m => m.Editor),
  },
  {
    path: '', canActivate: [authGuard],
    loadComponent: () => import('./admin/shell').then(m => m.Shell),
    children: [
      { path: '', loadComponent: () => import('./admin/dashboard').then(m => m.Dashboard) },
      { path: 'pages', canActivate: [capGuard('edit_pages')], data: { type: 'page' }, loadComponent: () => import('./admin/content-list').then(m => m.ContentList) },
      { path: 'posts', canActivate: [capGuard('edit_own_posts')], data: { type: 'post' }, loadComponent: () => import('./admin/content-list').then(m => m.ContentList) },
      { path: 'posts/new', canActivate: [capGuard('edit_own_posts')], loadComponent: () => import('./admin/post-edit').then(m => m.PostEdit) },
      { path: 'posts/:id', canActivate: [capGuard('edit_own_posts')], loadComponent: () => import('./admin/post-edit').then(m => m.PostEdit) },
      // post types of the site code (site.postType): the same list / editor screens with the type as input
      { path: 'content/:type', canActivate: [postTypeGuard], loadComponent: () => import('./admin/content-list').then(m => m.ContentList) },
      { path: 'content/:type/new', canActivate: [postTypeGuard], loadComponent: () => import('./admin/post-edit').then(m => m.PostEdit) },
      { path: 'content/:type/:id', canActivate: [postTypeGuard], loadComponent: () => import('./admin/post-edit').then(m => m.PostEdit) },
      { path: 'templates', canActivate: [capGuard('edit_pages')], data: { type: 'template' }, loadComponent: () => import('./admin/content-list').then(m => m.ContentList) },
      { path: 'media', canActivate: [capGuard('upload_files')], loadComponent: () => import('./admin/media-page').then(m => m.MediaPage) },
      { path: 'categories', canActivate: [capGuard('manage_categories')], loadComponent: () => import('./admin/manage-pages').then(m => m.Categories) },
      { path: 'menus', canActivate: [capGuard('edit_pages')], loadComponent: () => import('./admin/manage-pages').then(m => m.Menus) },
      { path: 'components', canActivate: [capGuard('edit_pages')], loadComponent: () => import('./admin/manage-pages').then(m => m.Components) },
      { path: 'users', canActivate: [capGuard('manage_users')], loadComponent: () => import('./admin/users-page').then(m => m.Users) },
      { path: 'roles', canActivate: [capGuard('manage_users')], loadComponent: () => import('./admin/users-page').then(m => m.Roles) },
      { path: 'settings', canActivate: [capGuard('manage_options')], loadComponent: () => import('./admin/settings-page').then(m => m.SettingsPage) },
      { path: 'plugins', canActivate: [capGuard('manage_options')], loadComponent: () => import('./admin/plugins-page').then(m => m.PluginsPage) },
      { path: 'p/:id', canActivate: [capGuard('manage_options')], data: { kind: 'builtin' }, loadComponent: () => import('./admin/plugin-settings').then(m => m.PluginSettings) },
      { path: 'x/:id/:page', canActivate: [pluginPageGuard], loadComponent: () => import('./admin/service-page').then(m => m.ServicePage) },
      { path: 'packages/:id', canActivate: [capGuard('manage_options')], data: { kind: 'package' }, loadComponent: () => import('./admin/plugin-settings').then(m => m.PluginSettings) },
      // Export / import site: a core system tool since phase 7 (was the backup plugin at /backup)
      { path: 'tools/export', canActivate: [capGuard('manage_options')], loadComponent: () => import('./admin/tools-export').then(m => m.ToolsExport) },
      { path: 'backup', redirectTo: 'tools/export' },
      { path: 'code', canActivate: [capGuard('view_scripts')], canDeactivate: [(c: { canLeave(): Promise<boolean> }) => c.canLeave()], loadComponent: () => import('./code/code-editor').then(m => m.CodeEditor) },
      { path: 'code/versions', canActivate: [capGuard('view_scripts')], loadComponent: () => import('./code/code-versions').then(m => m.CodeVersions) },
      { path: 'code/db', canActivate: [capGuard('view_code_data')], loadComponent: () => import('./code/db-overview').then(m => m.DbOverview) },
      { path: 'code/db/tables/:name', canActivate: [capGuard('view_code_data')], loadComponent: () => import('./code/db-table').then(m => m.DbTable) },
      { path: 'code/db/er', canActivate: [capGuard('view_code_data')], loadComponent: () => import('./code/db-er').then(m => m.DbEr) },
      { path: 'code/db/sql', canActivate: [capGuard('view_code_data')], loadComponent: () => import('./code/db-sql').then(m => m.DbSql) },
      { path: 'code/db/migrations', canActivate: [capGuard('view_code_data')], loadComponent: () => import('./code/db-migrations').then(m => m.DbMigrations) },
      { path: 'code/logs', canActivate: [capGuard('view_scripts')], loadComponent: () => import('./code/code-logs').then(m => m.CodeLogs) },
      { path: 'code/settings', canActivate: [capGuard('manage_scripts')], loadComponent: () => import('./code/code-settings').then(m => m.CodeSettingsPage) },
      { path: 'code/jobs', canActivate: [capGuard('view_scripts')], loadComponent: () => import('./code/code-jobs').then(m => m.CodeJobs) },
      { path: 'code/hooks', canActivate: [capGuard('view_scripts')], loadComponent: () => import('./code/code-hooks').then(m => m.CodeHooks) },
      { path: 'code/formatters', canActivate: [capGuard('view_scripts')], loadComponent: () => import('./code/code-formatters').then(m => m.CodeFormatters) },
      { path: 'code/access', canActivate: [capGuard('manage_scripts')], loadComponent: () => import('./code/code-access').then(m => m.CodeAccess) },
      { path: 'profile', loadComponent: () => import('./admin/users-page').then(m => m.Profile) },
    ],
  },
  { path: '**', redirectTo: '' },
];
