import { Component, inject } from '@angular/core';
import { SiteInfo } from './core/site';
import { RouterOutlet } from '@angular/router';
// Every translation table, registered up front so no screen can show a raw key.
import './admin/messages-admin';
import './admin/messages-platform';
import './editor/messages-editor';

@Component({
  selector: 'app-root',
  imports: [RouterOutlet],
  template: '<router-outlet />',
})
export class App {
  /** Site title + icon on every screen (login, designer, admin) from the public site info. */
  constructor() { inject(SiteInfo).loadPublic(); }
}
