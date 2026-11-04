import { inject } from '@angular/core';
import { tr } from '../core/i18n';
import './messages-editor4';
import { EditorStore } from './editor-store';
import { GuidePrefs, clearGuides, hasGuides } from './guides-model';
import { exportCommand, vectorCommands } from './vector-ops';

/** One editor action: shared by the main menu, right-click menu, shortcuts sheet and keyboard. */
export interface Command {
  id: string;
  label: string;
  keys?: string;
  run: () => void;
  enabled?: () => boolean;
  /** Toggle commands show a check mark. */
  checked?: () => boolean;
}

export interface CommandHost {
  store: EditorStore;
  save(status?: 'draft' | 'publish'): void;
  preview(): void;
  back(): void;
  fit(): void;
  zoomToSelection(): void;
  zoomBy(f: number): void;
  openShortcuts(): void;
  createComponent(): void;
  pickImage(): void;
  openFind(): void;
}

/** Figma's command set and shortcuts, grouped like Figma's main menu. */
export function buildCommands(h: CommandHost): Record<string, Command[]> {
  const s = h.store;
  const gp = inject(GuidePrefs); // buildCommands runs in the editor's field initializer (injection context)
  const has = () => s.selectedIds().length > 0;
  const one = () => s.selectedIds().length === 1;
  const child = () => !!s.selected()?.parent;
  const frameSel = () => child() && s.selected()?.node.type === 'frame' && !!s.selected()?.node.children?.length;
  const inst = () => s.selected()?.node.type === 'instance';
  return localize({
    File: [
      { id: 'save', label: 'Save', keys: 'Ctrl+S', run: () => h.save() },
      { id: 'publish', label: 'Publish', keys: 'Ctrl+Shift+P', run: () => h.save('publish'), enabled: () => s.kind() === 'post' },
      { id: 'preview', label: 'Preview in browser', keys: 'Ctrl+Alt+Enter', run: () => h.preview(), enabled: () => s.kind() === 'post' },
      exportCommand(s),
      { id: 'backHome', label: 'Back to dashboard', run: () => h.back() },
    ],
    Edit: [
      { id: 'undo', label: 'Undo', keys: 'Ctrl+Z', run: () => s.undo(), enabled: () => s.canUndo() },
      { id: 'redo', label: 'Redo', keys: 'Ctrl+Shift+Z', run: () => s.redo(), enabled: () => s.canRedo() },
      { id: 'copy', label: 'Copy', keys: 'Ctrl+C', run: () => s.copy(), enabled: has },
      { id: 'cut', label: 'Cut', keys: 'Ctrl+X', run: () => s.cut(), enabled: has },
      { id: 'paste', label: 'Paste', keys: 'Ctrl+V', run: () => s.paste() },
      { id: 'pasteInPlace', label: 'Paste in place', keys: 'Ctrl+Shift+V', run: () => s.paste(true) },
      { id: 'duplicate', label: 'Duplicate', keys: 'Ctrl+D', run: () => s.duplicateMany(), enabled: has },
      { id: 'delete', label: 'Delete', keys: 'Delete', run: () => s.removeMany(), enabled: has },
      { id: 'copyProps', label: 'Copy properties', keys: 'Ctrl+Alt+C', run: () => s.copyProps(), enabled: one },
      { id: 'pasteProps', label: 'Paste properties', keys: 'Ctrl+Alt+V', run: () => s.pasteProps(), enabled: has },
      { id: 'selectAll', label: 'Select all', keys: 'Ctrl+A', run: () => s.selectAll() },
      { id: 'selectMatching', label: 'Select all matching layers', keys: 'Ctrl+Alt+A', run: () => s.selectMatching(), enabled: one },
      { id: 'find', label: 'Find and replace…', keys: 'Ctrl+F', run: () => h.openFind() },
      { id: 'selectNone', label: 'Select none', keys: 'Esc', run: () => s.select(null), enabled: has },
      { id: 'rename', label: 'Rename', keys: 'Ctrl+R / F2', run: () => s.renamingId.set(s.selectedId()), enabled: one },
    ],
    View: [
      { id: 'zoomIn', label: 'Zoom in', keys: 'Ctrl++', run: () => h.zoomBy(1.25) },
      { id: 'zoomOut', label: 'Zoom out', keys: 'Ctrl+-', run: () => h.zoomBy(0.8) },
      { id: 'zoom100', label: 'Zoom to 100%', keys: 'Shift+0', run: () => s.zoom.set(1) },
      { id: 'zoomFit', label: 'Zoom to fit', keys: 'Shift+1', run: () => h.fit() },
      { id: 'zoomSel', label: 'Zoom to selection', keys: 'Shift+2', run: () => h.zoomToSelection(), enabled: has },
      { id: 'freeDrag', label: 'Free move (drop anywhere)', run: () => s.setFreeDrag(!s.freeDrag()), checked: () => s.freeDrag() },
      { id: 'deepSelect', label: 'Click selects deepest layer', run: () => s.setDeepSelect(!s.deepSelect()), checked: () => s.deepSelect() },
      { id: 'rulers', label: 'Rulers', keys: 'Shift+R', run: () => gp.toggle('rulers'), checked: () => gp.rulers() },
      { id: 'showGuides', label: 'Show guides', run: () => gp.toggle('guides'), checked: () => gp.guides() },
      { id: 'clearGuides', label: 'Clear guides', run: () => clearGuides(s), enabled: () => hasGuides(s) },
      { id: 'layoutGrids', label: 'Layout grids', keys: 'Ctrl+Shift+4', run: () => gp.toggle('grids'), checked: () => gp.grids() },
      { id: 'desktop', label: 'Desktop breakpoint', keys: 'Alt+1', run: () => s.bp.set('desktop') },
      { id: 'tablet', label: 'Tablet breakpoint', keys: 'Alt+2', run: () => s.bp.set('tablet') },
      { id: 'mobile', label: 'Mobile breakpoint', keys: 'Alt+3', run: () => s.bp.set('mobile') },
    ],
    Object: [
      { id: 'group', label: 'Group selection', keys: 'Ctrl+G', run: () => s.group(), enabled: child },
      { id: 'ungroup', label: 'Ungroup selection', keys: 'Ctrl+Shift+G', run: () => s.ungroup(), enabled: frameSel },
      { id: 'frame', label: 'Frame selection', keys: 'Ctrl+Alt+G', run: () => s.group('Frame'), enabled: child },
      { id: 'autoLayout', label: 'Add auto layout', keys: 'Shift+A', run: () => s.addAutoLayout(), enabled: has },
      { id: 'removeAutoLayout', label: 'Remove auto layout', keys: 'Alt+Shift+A', run: () => s.removeAutoLayout(), enabled: one },
      { id: 'component', label: 'Create component', keys: 'Ctrl+Alt+K', run: () => h.createComponent(), enabled: () => one() && child() && !inst() },
      { id: 'detach', label: 'Detach instance', keys: 'Ctrl+Alt+B', run: () => s.detachInstance(s.selectedId()!), enabled: inst },
      { id: 'front', label: 'Bring to front', keys: 'Ctrl+Shift+]', run: () => s.reorder('front'), enabled: child },
      { id: 'forward', label: 'Bring forward', keys: 'Ctrl+]', run: () => s.reorder('forward'), enabled: child },
      { id: 'backward', label: 'Send backward', keys: 'Ctrl+[', run: () => s.reorder('backward'), enabled: child },
      { id: 'back', label: 'Send to back', keys: 'Ctrl+Shift+[', run: () => s.reorder('back'), enabled: child },
      { id: 'hide', label: 'Show/Hide selection', keys: 'Ctrl+Shift+H', run: () => s.toggleVisible(), enabled: has },
      { id: 'lock', label: 'Lock/Unlock selection', keys: 'Ctrl+Shift+L', run: () => s.toggleLock(), enabled: has },
      { id: 'alignLeft', label: 'Align left', keys: 'Alt+A', run: () => s.align('left'), enabled: child },
      { id: 'alignHC', label: 'Align horizontal centers', keys: 'Alt+H', run: () => s.align('hcenter'), enabled: child },
      { id: 'alignRight', label: 'Align right', keys: 'Alt+D', run: () => s.align('right'), enabled: child },
      { id: 'alignTop', label: 'Align top', keys: 'Alt+W', run: () => s.align('top'), enabled: child },
      { id: 'alignVC', label: 'Align vertical centers', keys: 'Alt+V', run: () => s.align('vcenter'), enabled: child },
      { id: 'alignBottom', label: 'Align bottom', keys: 'Alt+S', run: () => s.align('bottom'), enabled: child },
      { id: 'distH', label: 'Distribute horizontal spacing', keys: 'Alt+Shift+H', run: () => s.distribute('h') },
      { id: 'distV', label: 'Distribute vertical spacing', keys: 'Alt+Shift+V', run: () => s.distribute('v') },
      ...vectorCommands(s), // union / subtract / intersect / exclude / flatten / edit path
    ],
    Tools: [
      { id: 'move', label: 'Move', keys: 'V', run: () => s.tool.set('move') },
      { id: 'hand', label: 'Hand tool', keys: 'H / Space', run: () => s.tool.set('hand') },
      { id: 'toolFrame', label: 'Frame', keys: 'F / A', run: () => s.tool.set('frame') },
      { id: 'toolRect', label: 'Rectangle', keys: 'R', run: () => s.tool.set('rect') },
      { id: 'toolEllipse', label: 'Ellipse', keys: 'O', run: () => s.tool.set('ellipse') },
      { id: 'toolText', label: 'Text', keys: 'T', run: () => s.tool.set('text') },
      { id: 'toolPen', label: 'Pen', keys: 'P', run: () => s.tool.set('pen') },
      { id: 'image', label: 'Place image…', keys: 'Ctrl+Shift+K', run: () => h.pickImage() },
    ],
    Help: [
      { id: 'shortcuts', label: 'Keyboard shortcuts', keys: 'Ctrl+/', run: () => h.openShortcuts() },
    ],
  });
}

/**
 * Labels follow the UI language: a command with an `ed.cmd.<id>` message reads
 * it on every access (the English label above is only a fallback). Group names
 * (File, Edit…) stay as keys; menus show them with `ed.menu.<group>`.
 */
function localize(groups: Record<string, Command[]>): Record<string, Command[]> {
  for (const list of Object.values(groups)) {
    for (const c of list) {
      const key = 'ed.cmd.' + c.id;
      const own = Object.getOwnPropertyDescriptor(c, 'label');
      if (own?.get) continue; // already translated (vector commands)
      const fallback = c.label;
      Object.defineProperty(c, 'label', { get: () => { const v = tr(key); return v === key ? fallback : v; }, enumerable: true, configurable: true });
    }
  }
  return groups;
}

/** Selection-related commands shown in the right-click menu (Figma order). */
export const CONTEXT_MENU: (string | '-')[] = [
  'copy', 'cut', 'paste', 'pasteInPlace', '-', 'duplicate', 'delete', '-',
  'front', 'forward', 'backward', 'back', '-',
  'group', 'ungroup', 'frame', 'autoLayout', '-',
  'component', 'detach', '-', 'hide', 'lock', '-', 'copyProps', 'pasteProps',
  '-', 'union', 'subtract', 'intersect', 'exclude', 'flatten', 'exportSel',
];
