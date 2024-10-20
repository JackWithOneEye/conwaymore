import './wasm_exec.js';

/**
 * @param {OffscreenCanvasRenderingContext2D} ctx 
 * @param {number} strokeStyle 
 * @param {number} lineWidth 
 */
globalThis.prepareCtx = (ctx, strokeStyle, lineWidth) => {
    ctx.clearRect(0, 0, ctx.canvas.width, ctx.canvas.height);
    ctx.beginPath()
    ctx.strokeStyle = `#${strokeStyle.toString(16).padStart(6, '0')}`;
    ctx.lineWidth = lineWidth;
}

/**
 * @param {OffscreenCanvas} canvas 
 * @param {number} w
 * @param {number} h 
 */
globalThis.setDimensions = (canvas, w, h) => {
    canvas.width = w;
    canvas.height = h;
}

/**
 * @param {OffscreenCanvasRenderingContext2D} ctx 
 * @param {number} x 
 * @param {number} len 
 */
globalThis.vertPath = (ctx, x, len) => {
    ctx.moveTo(x, 0);
    ctx.lineTo(x, len);
}

/**
 * @param {OffscreenCanvasRenderingContext2D} ctx 
 * @param {number} y 
 * @param {number} len 
 */
globalThis.horizPath = (ctx, y, len) => {
    ctx.moveTo(0, y);
    ctx.lineTo(len, y);
}

/**
 * @param {OffscreenCanvasRenderingContext2D} ctx 
 * @param {number} x
 * @param {number} y 
 * @param {number} w 
 * @param {number} h 
 */
globalThis.strokeAndFillRect = (ctx, x, y, w, h) => {
    ctx.strokeRect(x, y, w, h);
    ctx.fillRect(x, y, w, h);
}

// @ts-ignore
const go = new Go();
const { instance } = await WebAssembly.instantiateStreaming(fetch('go.wasm'), go.importObject);
await go.run(instance);

console.log("BYE!")

