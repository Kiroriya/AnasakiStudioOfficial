import { describe, expect, it } from 'vitest';
import { langLabel, parseLangs } from './languages';

describe('content languages', () => {
  it('parses the option, skipping bad codes, repeats and the site language', () => {
    expect(parseLangs('en, ja ,EN, th, x, zh-tw', 'th')).toEqual(['en', 'ja', 'zh-tw']);
    expect(parseLangs('', 'th')).toEqual([]);
  });
  it('names languages', () => {
    expect(langLabel('ja')).toBe('日本語');
    expect(langLabel('xx')).toBe('XX');
  });
});
