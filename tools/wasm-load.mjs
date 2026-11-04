// Loads the WASM sim under Node.
import {execFileSync} from 'node:child_process'
import {readFileSync} from 'node:fs'
import {createRequire} from 'node:module'

/** Instantiates main.wasm and returns the sim API, live on globalThis too. */
export async function loadSim(wasmPath) {
  // wasm_exec.js is a plain script that assigns globalThis.Go; require runs it
  // for that side effect and caches it.
  const goroot =
    process.env.GOROOT || execFileSync('go', ['env', 'GOROOT'], {encoding: 'utf8'}).trim()
  createRequire(import.meta.url)(`${goroot}/lib/wasm/wasm_exec.js`)

  const go = new Go()
  const {instance} = await WebAssembly.instantiate(readFileSync(wasmPath), go.importObject)

  // Go's main blocks on quit(), so this resolves at the end, not here.
  void go.run(instance)

  return globalThis.sim
}
