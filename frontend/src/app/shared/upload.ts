import { Api, errorMessage } from '../core/api';
import { tr } from '../core/i18n';
import { Media } from '../core/models';
import { mapLimit } from './concurrency';
import '../admin/messages-admin';

/** Uploads at once: enough to be quick, few enough to stay kind to the server. */
const UPLOAD_CONCURRENCY = 3;

/**
 * Uploads every file, a few at a time; a failed file doesn't stop the others.
 * `error` summarises the files that failed ('' when all went up).
 */
export async function uploadFiles(api: Api, files: File[]): Promise<{ uploaded: Media[]; error: string }> {
  const res = await mapLimit(files, UPLOAD_CONCURRENCY, f => api.upload(f));
  const uploaded: Media[] = [];
  const failed: string[] = [];
  res.forEach((r, i) => {
    if (r.status === 'fulfilled') uploaded.push(r.value);
    else failed.push(`${files[i].name} (${errorMessage(r.reason)})`);
  });
  const error = failed.length ? tr('media.uploadFailed', { n: failed.length, total: files.length, list: failed.join('; ') }) : '';
  return { uploaded, error };
}
