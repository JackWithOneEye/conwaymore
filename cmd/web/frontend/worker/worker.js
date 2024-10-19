import './wasm_exec.js';

// @ts-ignore
const go = new Go();
const { instance } = await WebAssembly.instantiateStreaming(fetch('go.wasm'), go.importObject);
await go.run(instance);

console.log("BYE!")
