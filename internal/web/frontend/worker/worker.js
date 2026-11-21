// @ts-expect-error wasm_exec.js is copied next to the bundled worker at build time.
import './wasm_exec.js';

// Called from Go (cmd/wasm) via js.Global().
Object.assign(globalThis, {
  /**
   * @param {OffscreenCanvas} canvas
   * @param {number} w
   * @param {number} h
   */
  setDimensions: (canvas, w, h) => {
    canvas.width = w;
    canvas.height = h;
  },

  /**
   * Upload dirty rows to bufCanvas if needed, tile bufCanvas across the full
   * viewport so the toroidal world wraps seamlessly on wide/tall screens, and
   * optionally draw the grid across the full canvas in one JS call.
   * @param {OffscreenCanvasRenderingContext2D} ctx
   * @param {OffscreenCanvas} bufCanvas
   * @param {OffscreenCanvasRenderingContext2D} bufCtx
   * @param {ImageData} imageData
   * @param {boolean} uploadBuf
   * @param {number} dirtyY
   * @param {number} dirtyW
   * @param {number} dirtyH
   * @param {number} cols
   * @param {number} rows
   * @param {number} startX
   * @param {number} startY
   * @param {number} tileW
   * @param {number} tileH
   * @param {number} canvasW
   * @param {number} canvasH
   * @param {boolean} showGrid
   * @param {number} gridX
   * @param {number} gridY
   * @param {number} cellSize
   * @param {string} gridColour
   * @param {number} gridLineWidth
   */
  renderFrame: (
    ctx,
    bufCanvas,
    bufCtx,
    imageData,
    uploadBuf,
    dirtyY,
    dirtyW,
    dirtyH,
    cols,
    rows,
    startX,
    startY,
    tileW,
    tileH,
    canvasW,
    canvasH,
    showGrid,
    gridX,
    gridY,
    cellSize,
    gridColour,
    gridLineWidth,
  ) => {
    if (uploadBuf) {
      bufCtx.putImageData(imageData, 0, 0, 0, dirtyY, dirtyW, dirtyH);
    }

    ctx.imageSmoothingEnabled = false;
    for (let ty = startY; ty < canvasH; ty += tileH) {
      for (let tx = startX; tx < canvasW; tx += tileW) {
        ctx.drawImage(bufCanvas, 0, 0, cols, rows, tx, ty, tileW, tileH);
      }
    }

    if (showGrid) {
      const half = gridLineWidth * 0.5;
      ctx.beginPath();
      ctx.strokeStyle = gridColour;
      ctx.lineWidth = gridLineWidth;
      for (let x = gridX; x <= canvasW; x += cellSize) {
        const px = x + half;
        ctx.moveTo(px, 0);
        ctx.lineTo(px, canvasH);
      }
      for (let y = gridY; y <= canvasH; y += cellSize) {
        const py = y + half;
        ctx.moveTo(0, py);
        ctx.lineTo(canvasW, py);
      }
      ctx.stroke();
    }
  },
});

// @ts-ignore
const go = new Go();
const { instance } = await WebAssembly.instantiateStreaming(fetch('go.wasm'), go.importObject);
await go.run(instance);

console.log("BYE!")
