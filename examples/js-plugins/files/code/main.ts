// The "files" unit (code.entry). Most of the plugin is the library (code.library = code/lib/index.ts):
// toCSV, toXLSX, toPDF and the download helpers are bundled into the importer and run with the
// importer's scopes and access rules.
//
// This unit adds one export, renderThaiPdf(spec): toPDF with the Sarabun font of THIS package
// (fonts/Sarabun-Regular.woff — site.asset reads the calling unit's own package files, so it has to run
// here, not in the importer). Importers use it through the library helpers thaiPdf / thaiPdfDownload
// (plugins.call), or directly: plugins.call('files', 'renderThaiPdf', spec).
import { toPDF, PdfSpec } from './lib/pdf';
import { parseTTF, TrueTypeFont } from './lib/ttf';

const FONT = 'fonts/Sarabun-Regular.woff';
let sarabun: TrueTypeFont | null = null; // parsed once per runtime

site.export('renderThaiPdf', (spec: PdfSpec) => {
  if (!spec || typeof spec !== 'object') throw new Error('renderThaiPdf(spec): spec must be an object');
  if (!sarabun) sarabun = parseTTF(site.asset(FONT).bytes());
  return toPDF({ ...spec, font: sarabun });
});
