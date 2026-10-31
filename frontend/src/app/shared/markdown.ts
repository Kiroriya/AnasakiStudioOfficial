/**
 * Tiny, safe markdown → HTML for plugin readmes (headings, lists, bold, italic,
 * inline code, code blocks, links). Input is escaped first, so no raw HTML gets through.
 */
export function markdown(src: string): string {
  const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;').replace(/"/g, '&quot;');
  const inline = (s: string) =>
    esc(s)
      .replace(/`([^`]+)`/g, '<code>$1</code>')
      .replace(/\*\*([^*]+)\*\*/g, '<strong>$1</strong>')
      .replace(/(^|[^*])\*([^*]+)\*/g, '$1<em>$2</em>')
      .replace(/\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g, '<a href="$2" target="_blank" rel="noopener">$1</a>');
  const out: string[] = [];
  let list: 'ul' | 'ol' | null = null;
  let code: string[] | null = null;
  const closeList = () => { if (list) { out.push(`</${list}>`); list = null; } };
  for (const line of src.replace(/\r/g, '').split('\n')) {
    if (line.startsWith('```')) {
      if (code) { out.push(`<pre><code>${esc(code.join('\n'))}</code></pre>`); code = null; } else { closeList(); code = []; }
      continue;
    }
    if (code) { code.push(line); continue; }
    const h = /^(#{1,4})\s+(.*)$/.exec(line);
    const ul = /^\s*[-*]\s+(.*)$/.exec(line);
    const ol = /^\s*\d+\.\s+(.*)$/.exec(line);
    if (h) { closeList(); const lv = Math.min(6, h[1].length + 2); out.push(`<h${lv}>${inline(h[2])}</h${lv}>`); }
    else if (ul || ol) {
      const kind = ul ? 'ul' : 'ol';
      if (list !== kind) { closeList(); out.push(`<${kind}>`); list = kind; }
      out.push(`<li>${inline((ul ?? ol)![1])}</li>`);
    } else if (!line.trim()) closeList();
    else { closeList(); out.push(`<p>${inline(line)}</p>`); }
  }
  if (code) out.push(`<pre><code>${esc(code.join('\n'))}</code></pre>`);
  closeList();
  return out.join('\n');
}
