// Runs a single-player game's engine (cmd/wasmengine) on its own thread.
// @ts-expect-error wasm_exec.js is copied next to the bundled worker at build time.
import './wasm_exec.js';

// @ts-ignore
const go = new Go();
const { instance } = await WebAssembly.instantiateStreaming(fetch('engine.wasm'), go.importObject);
await go.run(instance);
