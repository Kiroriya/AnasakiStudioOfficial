/**
 * Saves a blob as a file download. The object URL is revoked a little later, not right
 * after click(): revoking it synchronously can cancel the download in some browsers.
 */
export function saveBlob(blob: Blob, filename: string) {
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = filename;
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 30_000);
}
