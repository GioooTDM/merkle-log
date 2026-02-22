// verify.js (ESM) — Tessera/RFC6962 inclusion proof verification (offline)
// Assumes proof JSON contains: { log_index, tree_size, root_hash, checkpoint, proof:[hex...] }
// Assumes event JSON file is EXACTLY the bytes appended in the log (server used tessera.NewEntry(body)).

const $ = (id) => document.getElementById(id);

const statusEl = $("verifyStatus");
const detailsEl = $("proofDetails");

const resDocUid = $("resDocUid");
const resVersion = $("resVersion");
const resDate = $("resDate");
const resIndex = $("resIndex");

function setStatus(kind, msg) {
  statusEl.className = `status-box ${kind}`;
  statusEl.textContent = msg;
}

function hexToBytes(hex) {
  const h = hex.startsWith("0x") ? hex.slice(2) : hex;
  if (!/^[0-9a-fA-F]*$/.test(h)) throw new Error("invalid hex");
  if (h.length % 2 !== 0) throw new Error("hex length must be even");
  const out = new Uint8Array(h.length / 2);
  for (let i = 0; i < out.length; i++) out[i] = parseInt(h.slice(i * 2, i * 2 + 2), 16);
  return out;
}

function bytesToHex(bytes) {
  return Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
}

async function sha256(bytes) {
  const buf = await crypto.subtle.digest("SHA-256", bytes);
  return new Uint8Array(buf);
}

// RFC6962: leaf = SHA256(0x00 || leafBytes)
async function hashLeaf(leafBytes) {
  const pref = new Uint8Array(1 + leafBytes.length);
  pref[0] = 0x00;
  pref.set(leafBytes, 1);
  return sha256(pref);
}

// RFC6962: node = SHA256(0x01 || left || right)
async function hashNode(left32, right32) {
  if (left32.length !== 32 || right32.length !== 32) throw new Error("hashNode needs 32-byte hashes");
  const pref = new Uint8Array(1 + 32 + 32);
  pref[0] = 0x01;
  pref.set(left32, 1);
  pref.set(right32, 1 + 32);
  return sha256(pref);
}

/**
 * Verifica inclusion proof RFC6962 (gestisce tree_size non potenza di 2).
 * proofHex: sibling hashes (hex) nell'ordine standard (dal basso verso l'alto).
 * L/R è implicito e dipende da index + treeSize.
 */
async function verifyInclusionRFC6962({ index, treeSize, rootHex, leafBytes, proofHex }) {
  if (!Number.isSafeInteger(index) || index < 0) throw new Error("index invalid");
  if (!Number.isSafeInteger(treeSize) || treeSize <= 0) throw new Error("treeSize invalid");
  if (index >= treeSize) throw new Error("index >= treeSize");

  const root = hexToBytes(rootHex);
  if (root.length !== 32) throw new Error("root_hash must be 32 bytes hex");

  const sibs = proofHex.map((h, i) => {
    const b = hexToBytes(h);
    if (b.length !== 32) throw new Error(`proof[${i}] not 32 bytes`);
    return b;
  });

  let r = await hashLeaf(leafBytes);
  let idx = BigInt(index);
  let n = BigInt(treeSize);

  let pi = 0;

  // RFC6962 uneven-tree climb:
  // - if idx is right child => consume left sibling
  // - else if idx is left child AND not last node => consume right sibling
  // - else (orphan) => consume nothing
  while (n > 1n) {
    if ((idx & 1n) === 1n) {
      if (pi >= sibs.length) throw new Error("proof too short");
      r = await hashNode(sibs[pi++], r);
    } else if (idx < n - 1n) {
      if (pi >= sibs.length) throw new Error("proof too short");
      r = await hashNode(r, sibs[pi++]);
    }
    idx >>= 1n;
    n = (n + 1n) >> 1n;
  }

  if (pi !== sibs.length) throw new Error("proof too long");

  const computedRootHex = bytesToHex(r);
  return { ok: computedRootHex === bytesToHex(root), computedRootHex };
}

async function readFileAsBytes(file) {
  const buf = await file.arrayBuffer();
  return new Uint8Array(buf);
}

async function readFileAsText(file) {
  return await file.text();
}

async function readJsonFromText(text) {
  return JSON.parse(text);
}

// Calcola SHA-256 nudo del documento (offline doc integrity)
async function hashDocumentBytes(docBytes) {
  const h = await sha256(docBytes);
  return bytesToHex(h);
}

function extractEventPayloadHash(eventJson) {
  const v = eventJson?.payload_hash?.value;
  if (typeof v !== "string") throw new Error("event.payload_hash.value missing");
  if (!v.startsWith("hex:")) throw new Error("event.payload_hash.value must start with 'hex:'");
  const hex = v.slice("hex:".length).toLowerCase();
  if (!/^[0-9a-f]+$/.test(hex)) throw new Error("event.payload_hash.value not hex");
  return hex;
}

function pickMeta(eventJson) {
  return {
    docUid: eventJson?.doc_uid ?? eventJson?.document_id ?? "-",
    version: eventJson?.doc_version ?? eventJson?.version ?? "-",
    date: eventJson?.recorded_at ?? eventJson?.issued_at ?? eventJson?.time ?? eventJson?.date ?? eventJson?.timestamp ?? "-",
  };
}

$("btnVerify").addEventListener("click", async () => {
  try {
    setStatus("status-idle", "Verifica in corso...");
    detailsEl.textContent = "(calcolo...)";

    const fDoc = $("verifyFile").files?.[0];
    const fEvent = $("verifyEvent").files?.[0];
    const fProof = $("verifyProof").files?.[0];

    if (!fDoc || !fEvent || !fProof) {
      setStatus("status-error", "Carica documento, evento e proof.");
      return;
    }

    // Leggi:
    const [docBytes, eventTextRaw, proofTextRaw] = await Promise.all([
      readFileAsBytes(fDoc),
      readFileAsText(fEvent),
      readFileAsText(fProof),
    ]);

    // Event bytes RAW = leafBytes (devono matchare il body appeso nel log)
    const eventBytes = new TextEncoder().encode(eventTextRaw);
    const eventJson = await readJsonFromText(eventTextRaw);
    const proofJson = await readJsonFromText(proofTextRaw);

    // 1) Documento vs payload_hash dell'evento
    const docHashHex = await hashDocumentBytes(docBytes);
    const expectedDocHashHex = extractEventPayloadHash(eventJson);
    const docOk = docHashHex === expectedDocHashHex;

    // 2) Proof inclusion
    const index = Number(proofJson.log_index);
    const treeSize = Number(proofJson.tree_size);
    const rootHex = String(proofJson.root_hash || "").toLowerCase();
    const proofHex = proofJson.proof;

    if (!/^[0-9a-f]{64}$/.test(rootHex)) throw new Error("proof.root_hash must be 32-byte hex");
    if (!Array.isArray(proofHex)) throw new Error("proof.proof must be array");
    const proofHexNorm = proofHex.map((x) => String(x).toLowerCase());

    const { ok: proofOk, computedRootHex } = await verifyInclusionRFC6962({
      index,
      treeSize,
      rootHex,
      leafBytes: eventBytes,
      proofHex: proofHexNorm,
    });

    // UI
    const meta = pickMeta(eventJson);
    resDocUid.textContent = String(meta.docUid);
    resVersion.textContent = String(meta.version);
    resDate.textContent = String(meta.date);
    resIndex.textContent = String(index);

    detailsEl.textContent = JSON.stringify(
      {
        doc_hash_computed: docHashHex,
        doc_hash_expected: expectedDocHashHex,
        doc_ok: docOk,

        proof_index: index,
        proof_tree_size: treeSize,
        root_hash_expected: rootHex,
        root_hash_computed: computedRootHex,
        proof_ok: proofOk,

        checkpoint_present: typeof proofJson.checkpoint === "string" && proofJson.checkpoint.length > 0,
      },
      null,
      2
    );

    if (docOk && proofOk) {
      setStatus("status-success", "✅ Verifica OK: documento coerente e proof valida.");
    } else if (!docOk && proofOk) {
      setStatus("status-error", "❌ Proof valida, ma il documento NON corrisponde all’hash nell’evento.");
    } else if (docOk && !proofOk) {
      setStatus("status-error", "❌ Documento OK, ma proof NON valida (root hash mismatch).");
    } else {
      setStatus("status-error", "❌ Documento e proof NON validi.");
    }
  } catch (e) {
    console.error(e);
    setStatus("status-error", `Errore: ${e?.message ?? String(e)}`);
    detailsEl.textContent = String(e?.stack ?? e);
  }
});
