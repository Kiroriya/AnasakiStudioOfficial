import { actionText } from './activity-page';

const t = (k: string) => k;

describe('activity log action text (2.4.0)', () => {
  it('names sign-ins by their result', () => {
    expect(actionText({ method: 'POST', route: 'POST /api/v1/auth/login', status: 200, via: 'login' }, t)).toBe('act.login');
    expect(actionText({ method: 'POST', route: 'POST /api/v1/auth/login', status: 401, via: 'login' }, t)).toBe('act.loginFailed');
    expect(actionText({ method: 'POST', route: 'POST /api/v1/auth/login', status: 429, via: 'login' }, t)).toBe('act.loginLocked');
  });
  it('turns a route into a verb and an object', () => {
    expect(actionText({ method: 'PUT', route: 'PUT /api/v1/posts/{id}', status: 200, via: 'session' }, t)).toBe('act.update · act.obj.posts');
    expect(actionText({ method: 'DELETE', route: 'DELETE /api/v1/users/{id}', status: 204, via: 'session' }, t)).toBe('act.delete · act.obj.users');
    expect(actionText({ method: 'POST', route: 'POST /api/v1/package-plugins/{id}/activate', status: 200, via: 'token' }, t)).toBe('act.activate · act.obj.plugins');
    expect(actionText({ method: 'POST', route: 'POST /api/v1/weird/thing', status: 200, via: 'session' }, t)).toBe('act.create · act.obj.other');
  });
});
