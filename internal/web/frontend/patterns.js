import { getElementByIdOrDie } from './helpers';

/** 
 * @typedef {{name: string; cells: {x: number, y: number}[], center_x: number; center_y: number}} Pattern
 */

/** 
 * @returns {ReadonlyMap<string, Pattern>}
 */
export function getPatterns() {
  const patternsEl = getElementByIdOrDie('patterns');
  if (!patternsEl.textContent) {
    throw new Error('patterns element is empty');
  }

  const patternsRaw = JSON.parse(patternsEl.textContent);
  /** @type {Map<string, Pattern>} */
  const patterns = new Map();
  for (const key in patternsRaw) {
    patterns.set(key, patternsRaw[key]);
  }
  return patterns;
}
