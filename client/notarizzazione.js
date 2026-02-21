// ===== Config =====
const ADD_URL = "http://localhost:2025/add";   // <- COSTANTE richiesta
const MAX_JSON_BYTES = 2048;                  // 2KB

// ===== DOM =====
const el = (id) => document.getElementById(id);
const fileEl = el("file");
const issuerIdEl = el("issuerId");
const issuerNameEl = el("issuerName");
const docUidEl = el("docUid");
const eventTypeEl = el("eventType");
const docVersionEl = el("docVersion");
const prevEventIdEl = el("prevEventId");
const titleEl = el("title");
const descriptionEl = el("description");
const notesEl = el("notes");

const btnBuild = el("btnBuild");
const btnSend = el("btnSend");
const btnDownload = el("btnDownload");

const outEl = el("out");
const respEl = el("resp");
const statusEl = el("status");

let lastCanonicalJSON = null;

// ===== Helpers =====
function setStatus(msg, kind = "ok") {
  statusEl.innerHTML = `<p class="${kind === "ok" ? "ok" : "err"}">${escapeHtml(msg)}</p>`;
}
function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ "&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;" }[c]));
}
function utf8Bytes(s) {
  return new TextEncoder().encode(s);
}
function nowRFC3339Nanoish() {
  // JS non ha nanos reali; ISO8601 con millis è ok per RFC3339 (e accettabile per il tuo parser).
  return new Date().toISOString();
}
function requireNonEmpty(name, v) {
  if (!v || !String(v).trim()) throw new Error(`${name} è richiesto`);
}
function isUUIDv4(s) {
  return /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/i.test(s);
}
function hexFromBytes(buf) {
  const b = new Uint8Array(buf);
  let out = "";
  for (let i = 0; i < b.length; i++) out += b[i].toString(16).padStart(2, "0");
  return out;
}

// Deterministic canonicalization (compact + stable key order).
// Per il tuo schema (stringhe + int) è sufficiente: sort keys ricorsivo + JSON.stringify.
function canonicalizeDeterministic(value) {
  const norm = normalizeForCanonical(value);
  return JSON.stringify(norm); // no spaces, stable because we already sorted keys
}

function normalizeForCanonical(v) {
  if (v === null) return null;

  const t = typeof v;
  if (t === "string") return v;
  if (t === "boolean") return v;
  if (t === "number") {
    if (!Number.isFinite(v)) throw new Error("Numeri non finiti non ammessi");
    // Per sicurezza: vieta float (RFC8785 numeri è delicato). Nel tuo schema: usa int.
    if (!Number.isInteger(v)) throw new Error("Numeri non interi non ammessi (usa interi)");
    // Safe integer only.
    if (!Number.isSafeInteger(v)) throw new Error("Intero fuori range JS (non safe integer)");
    return v;
  }

  if (Array.isArray(v)) return v.map(normalizeForCanonical);

  if (t === "object") {
    const out = {};
    const keys = Object.keys(v).filter((k) => v[k] !== undefined); // undefined non è JSON
    keys.sort(); // lexicographic
    for (const k of keys) out[k] = normalizeForCanonical(v[k]);
    return out;
  }

  throw new Error(`Tipo non supportato nel JSON: ${t}`);
}

async function sha256HexOfFile(file) {
  const ab = await file.arrayBuffer();
  const digest = await crypto.subtle.digest("SHA-256", ab);
  return hexFromBytes(digest);
}

function buildEventObject({ fileName, payloadHex }) {
  const issuerId = issuerIdEl.value.trim();
  const issuerName = issuerNameEl.value.trim();
  const docUid = docUidEl.value.trim();
  const eventType = eventTypeEl.value;
  const docVersion = Number(docVersionEl.value);
  const prevEventId = prevEventIdEl.value.trim();
  const title = titleEl.value.trim();
  const description = descriptionEl.value.trim();
  const notes = notesEl.value.trim();

  requireNonEmpty("issuer.entity_id", issuerId);
  requireNonEmpty("doc_uid", docUid);
  requireNonEmpty("title", title);
  if (!Number.isInteger(docVersion) || docVersion < 1) throw new Error("doc_version deve essere un intero >= 1");

  // prev_event_id policy
  const needsPrev = (eventType === "UPDATE" || eventType === "REVOKE" || eventType === "EXPIRE");
  if (needsPrev) {
    requireNonEmpty("prev_event_id", prevEventId);
    if (!isUUIDv4(prevEventId)) throw new Error("prev_event_id deve essere UUIDv4");
  } else {
    if (prevEventId) throw new Error("prev_event_id non deve essere presente per CREATE");
  }

  // payload_hash policy
  const needsPayload = (eventType === "CREATE" || eventType === "UPDATE");
  const payload_hash = needsPayload
    ? { alg: "sha-256", value: `hex:${payloadHex}` }
    : undefined;

  if (needsPayload && (!payloadHex || payloadHex.length !== 64)) {
    throw new Error("payload_hash sha-256 hex non valido");
  }

  const issuedAt = nowRFC3339Nanoish();
  const recordedAt = nowRFC3339Nanoish();

  const ev = {
    schema: "pa-notary-event@1",
    event_id: crypto.randomUUID(),
    event_type: eventType,
    doc_uid: docUid,
    doc_version: docVersion,
    // prev_event_id: null/absent for CREATE; required otherwise
    ...(needsPrev ? { prev_event_id: prevEventId } : {}),
    ...(payload_hash ? { payload_hash } : {}),
    issuer: {
      entity_id: issuerId,
      ...(issuerName ? { name: issuerName } : {}),
    },
    issued_at: issuedAt,
    recorded_at: recordedAt,
    title,
    ...(description ? { description } : {}),
    ...(notes ? { notes } : {}),
    // Puoi aggiungere: file_name, prev_event_leaf, signature, ecc.
    // ...(fileName ? { file_name: fileName } : {}),
  };

  // Regola CREATE: doc_version=1 (come da spec)
  if (eventType === "CREATE" && docVersion !== 1) {
    throw new Error("CREATE richiede doc_version=1");
  }
  if (eventType === "UPDATE" && docVersion < 2) {
    throw new Error("UPDATE richiede doc_version>=2");
  }

  return ev;
}

// ===== UI actions =====
btnBuild.addEventListener("click", async () => {
  try {
    respEl.textContent = "(vuoto)";
    setStatus("Elaborazione...", "ok");

    const file = fileEl.files?.[0];
    if (!file) throw new Error("Seleziona un file");

    const payloadHex = await sha256HexOfFile(file);
    const ev = buildEventObject({ fileName: file.name, payloadHex });

    const canon = canonicalizeDeterministic(ev);
    const size = utf8Bytes(canon).length;
    if (size > MAX_JSON_BYTES) throw new Error(`JSON troppo grande: ${size} bytes (max ${MAX_JSON_BYTES})`);

    lastCanonicalJSON = canon;
    outEl.textContent = canon;

    btnSend.disabled = false;
    btnDownload.disabled = false;
    setStatus(`OK. JSON pronto (${size} bytes).`, "ok");
  } catch (e) {
    lastCanonicalJSON = null;
    outEl.textContent = "(vuoto)";
    btnSend.disabled = true;
    btnDownload.disabled = true;
    setStatus(e?.message || String(e), "err");
  }
});

btnSend.addEventListener("click", async () => {
  try {
    if (!lastCanonicalJSON) throw new Error("Prima genera il JSON");
    respEl.textContent = "(inviando...)";

    const res = await fetch(ADD_URL, {
      method: "POST",
      headers: { "content-type": "application/json" },
      body: lastCanonicalJSON,
    });

    const text = await res.text();
    if (!res.ok) throw new Error(`Server ${res.status}: ${text}`);
    respEl.textContent = text.trim();
    setStatus("Notarizzato con successo.", "ok");
  } catch (e) {
    respEl.textContent = "(errore)";
    setStatus(e?.message || String(e), "err");
  }
});

btnDownload.addEventListener("click", () => {
  try {
    if (!lastCanonicalJSON) throw new Error("Prima genera il JSON");
    const blob = new Blob([lastCanonicalJSON], { type: "application/json" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = "event.json";
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
  } catch (e) {
    setStatus(e?.message || String(e), "err");
  }
});

document.getElementById('btnMock').addEventListener('click', () => {
  const comuni = ["Roma", "Milano", "Napoli", "Torino", "Palermo", "Genova", "Bologna", "Firenze"];
  const tipiDoc = ["Determina", "Ordinanza", "Delibera", "Protocollo", "Certificato"];
  const possibiliNote = [
    "Generato automaticamente per test notarizzazione.",
    "Documento verificato dall'ufficio legale.",
    "Contiene allegati tecnici riservati.",
    "Notarizzazione prioritaria.",
    "In attesa di firma digitale aggiuntiva.",
    "Copia conforme all'originale cartaceo.",
    "" // Opzione vuota
  ];
  
  const comune = comuni[Math.floor(Math.random() * comuni.length)];
  const tipo = tipiDoc[Math.floor(Math.random() * tipiDoc.length)];
  const nota = possibiliNote[Math.floor(Math.random() * possibiliNote.length)];
  const anno = 2026;
  const num = Math.floor(Math.random() * 100000);

  // Selezione elementi e popolamento
  document.getElementById('issuerId').value = `IPA:c_${comune.toLowerCase().replace(' ', '')}`;
  document.getElementById('issuerName').value = `Comune di ${comune}`;
  document.getElementById('docUid').value = `${tipo.toUpperCase()}/${anno}/${num}`;
  
  // Fissi come richiesto
  document.getElementById('eventType').value = "CREATE";
  document.getElementById('docVersion').value = 1;
  document.getElementById('prevEventId').value = ""; // Vuoto per CREATE
  
  document.getElementById('title').value = `${tipo} dirigenziale n. ${num}`;
  document.getElementById('description').value = `Documento relativo alla gestione pratica ${num} per l'ufficio tecnico di ${comune}.`;
  document.getElementById('notes').value = nota;
});