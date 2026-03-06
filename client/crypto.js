/**
 * crypto.js — Primitive crittografiche condivise (RFC 6962 / SHA-256)
 *
 * Questo modulo centralizza le funzioni di hashing e conversione hex/bytes
 * che erano duplicate in verify.js, verify_consistency.html, recover_proof.html
 * e notarizzazione.js.
 *
 * Tutte le funzioni sono pure e non dipendono dal DOM.
 */

// ---------------------------------------------------------------------------
// Conversioni hex ↔ bytes
// ---------------------------------------------------------------------------

/**
 * Converte una stringa hex in Uint8Array.
 * Accetta prefisso opzionale "0x". Lunghezza qualsiasi (non solo 32 byte).
 * @param {string} hex
 * @returns {Uint8Array}
 */
export function hexToBytes(hex) {
  const h = String(hex || "").trim().replace(/^0x/, "");
  if (!/^[0-9a-fA-F]*$/.test(h)) throw new Error("hexToBytes: caratteri non hex");
  if (h.length % 2 !== 0) throw new Error("hexToBytes: lunghezza hex dispari");
  const out = new Uint8Array(h.length / 2);
  for (let i = 0; i < out.length; i++) {
    out[i] = parseInt(h.slice(i * 2, i * 2 + 2), 16);
  }
  return out;
}

/**
 * Converte Uint8Array (o array di byte) in stringa hex minuscola.
 * @param {Uint8Array|number[]} bytes
 * @returns {string}
 */
export function bytesToHex(bytes) {
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

/**
 * Converte una stringa base64 in Uint8Array.
 * Usata per decodificare il root hash nei checkpoint Tessera.
 * @param {string} b64
 * @returns {Uint8Array}
 */
export function b64ToBytes(b64) {
  const raw = atob(String(b64).trim());
  const out = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}

// ---------------------------------------------------------------------------
// SHA-256
// ---------------------------------------------------------------------------

/**
 * Calcola SHA-256 di un Uint8Array e restituisce Uint8Array (32 byte).
 * @param {Uint8Array} bytes
 * @returns {Promise<Uint8Array>}
 */
export async function sha256(bytes) {
  const buf = await crypto.subtle.digest("SHA-256", bytes);
  return new Uint8Array(buf);
}

/**
 * Calcola SHA-256 di dati arbitrari (Uint8Array, ArrayBuffer o stringa)
 * e restituisce la stringa hex risultante.
 *
 * Unifica sha256HexOfFile (notarizzazione.js) e calculateHash (recover_proof.html).
 *
 * @param {Uint8Array|ArrayBuffer|string} data
 * @returns {Promise<string>} hex string (64 caratteri)
 */
export async function sha256Hex(data) {
  let bytes;
  if (typeof data === "string") {
    bytes = new TextEncoder().encode(data);
  } else if (data instanceof ArrayBuffer) {
    bytes = new Uint8Array(data);
  } else {
    bytes = data; // già Uint8Array
  }
  const hash = await sha256(bytes);
  return bytesToHex(hash);
}

// ---------------------------------------------------------------------------
// RFC 6962 — nodi Merkle
// ---------------------------------------------------------------------------

/**
 * Hash foglia RFC 6962: SHA256(0x00 || leafBytes)
 * @param {Uint8Array} leafBytes
 * @returns {Promise<Uint8Array>} 32 byte
 */
export async function hashLeaf(leafBytes) {
  const input = new Uint8Array(1 + leafBytes.length);
  input[0] = 0x00;
  input.set(leafBytes, 1);
  return sha256(input);
}

/**
 * Hash nodo RFC 6962: SHA256(0x01 || left || right)
 * @param {Uint8Array} left32  — hash sinistro (32 byte)
 * @param {Uint8Array} right32 — hash destro (32 byte)
 * @returns {Promise<Uint8Array>} 32 byte
 */
export async function hashNode(left32, right32) {
  if (left32.length !== 32 || right32.length !== 32) {
    throw new Error("hashNode: entrambi gli input devono essere 32 byte");
  }
  const input = new Uint8Array(1 + 32 + 32);
  input[0] = 0x01;
  input.set(left32, 1);
  input.set(right32, 33);
  return sha256(input);
}

// ---------------------------------------------------------------------------
// Utilità byte
// ---------------------------------------------------------------------------

/**
 * Confronto costante tra due Uint8Array (byte per byte).
 * @param {Uint8Array} a
 * @param {Uint8Array} b
 * @returns {boolean}
 */
export function equalBytes(a, b) {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) {
    if (a[i] !== b[i]) return false;
  }
  return true;
}