/**
 * @param {TemplateStringsArray} strings
 * @returns {{bytes: Uint8Array; count: number}}
 */
function cs({ raw: [input] }) {
  let inRow = false;
  let y = -1;
  let x = -1;

  /** @type {number[]} */
  const bytes = [];
  let count = 0
  for (const char of input) {
    if (char === '|') {
      if (inRow) {
        inRow = false;
        x = -1;
      } else {
        inRow = true;
        y += 1;
      }
      continue;
    }

    if (!inRow) {
      continue;
    }

    x += 1;
    if (char === 'x') {
      bytes.push((x >> 8) & 0xff);
      bytes.push(x & 0xff);
      bytes.push((y >> 8) & 0xff);
      bytes.push(y & 0xff);
      count++;
    }
  }

  return { bytes: new Uint8Array(bytes), count };
}

export const Patterns = /** @type {const} */ ({
  'acorn': {
    name: 'Acorn',
    coordinates: cs`
      | x     |
      |   x   |
      |xx  xxx| 
    `
  },
  'bi-gun': {
    name: 'Bi-Gun',
    coordinates: cs`
      |           x                                      |
      |          xx                                      |
      |         xx                                       |
      |          xx  xx                                  |
      |                                      x           |
      |                                      xx        xx|
      |                                       xx       xx|
      |          xx  xx                  xx  xx          |
      |xx       xx                                       |
      |xx        xx                                      |
      |           x                                      |
      |                                  xx  xx          |
      |                                       xx         |
      |                                      xx          |
      |                                      x           |
    `
  },
  'blinker-puffer': {
    name: 'Blinker Puffer',
    coordinates: cs`
      |   x     |
      | x   x   |
      |x        |
      |x    x   |
      |xxxxx    |
      |         |
      |         |
      |         |
      | xx      |
      |xx xxx   |
      | xxxx    |
      |  xx     |
      |         |
      |     xx  |
      |   x    x|
      |  x      |
      |  x     x|
      |  xxxxxx |
    `
  },
  'glider': {
    name: 'Glider',
    coordinates: cs`
      |  x|
      |x x|
      | xx|
    `
  },
  'gosper-glider-gun': {
    name: 'Gosper Glider Gun',
    coordinates: cs`
      |                        x           |
      |                      x x           |
      |            xx      xx            xx|
      |           x   x    xx            xx|
      |xx        x     x   xx              |
      |xx        x   x xx    x x           |
      |          x     x       x           |
      |           x   x                    |
      |            xx                      |
    `
  },
  'light-weight-spaceship': {
    name: 'Light Weight Spaceship',
    coordinates: cs`
      | xxxx|
      |x   x|
      |    x|
      |x  x | 
    `
  },
  'middle-weight-spaceship': {
    name: 'Middle Weight Spaceship',
    coordinates: cs`
      |  x   |
      |x   x |
      |     x|
      |x    x|
      | xxxxx|
    `
  },
  'heavy-weight-spaceship': {
    name: 'Heavy Weight Spaceship',
    coordinates: cs`
      |  xx   |
      |x    x |
      |      x|
      |x     x|
      | xxxxxx|
    `
  },
  'noahs-ark': {
    name: 'Noah\'s Ark',
    coordinates: cs`
      |          x x  |
      |         x     |
      |          x  x |
      |            xxx|
      |               |
      |               |
      |               |
      |               |
      |               |
      | x             |
      |x x            |
      |               |
      |x  x           |
      |  xx           |
      |   x           |
    `
  },
  'pi-heptomino': {
    name: 'pi-Heptomino',
    coordinates: cs`
      |xxx|
      |x x|
      |x x|
    `
  },
  'r-pentomino': {
    name: 'r-Pentomino',
    coordinates: cs`
      | xx|
      |xx |
      | x |
    `
  },
  'switch-engine': {
    name: 'Switch Engine',
    coordinates: cs`
      | x x  |
      |x     |
      | x  x |
      |   xxx|
    `
  }
});

/** 
 * @typedef {keyof typeof Patterns} PatternType
 * @typedef {typeof Patterns[PatternType]} Pattern
 */
