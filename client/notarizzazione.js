import {
  API_BASE,
  DEMO_ISSUERS,
  el,
  downloadText,
} from "./common.js";

// ===== Config =====
const ADD_URL = `${API_BASE}/add`;
const MAX_JSON_BYTES = 2048;

// ===== DOM Elements =====
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

// ===== State =====
let lastRequestJSON = null;
let lastNotarizedJSON = null;

function populateIssuerOptions() {
  issuerIdEl.innerHTML = "";
  DEMO_ISSUERS.forEach((issuer) => {
    const option = document.createElement("option");
    option.value = issuer.entityId;
    option.textContent = issuer.entityId || "Seleziona issuer entity id...";
    issuerIdEl.appendChild(option);
  });
}

function syncIssuerName() {
  const selected = DEMO_ISSUERS.find((issuer) => issuer.entityId === issuerIdEl.value);
  issuerNameEl.value = selected ? selected.name : "";
}

// ===== Form Helpers =====
function getFormData() {
  return {
    issuerId: issuerIdEl.value.trim(),
    issuerName: issuerNameEl.value.trim(),
    docUid: docUidEl.value.trim(),
    eventType: eventTypeEl.value,
    docVersion: Number(docVersionEl.value),
    prevEventId: prevEventIdEl.value.trim(),
    title: titleEl.value.trim(),
    description: descriptionEl.value.trim(),
    notes: notesEl.value.trim(),
  };
}

// ===== UI & Formatting Helpers =====
function setStatus(msg, kind = "ok") {
  statusEl.innerHTML = `<p class="${kind === "ok" ? "ok" : "err"}">${escapeHtml(msg)}</p>`;
}

function escapeHtml(s) {
  return String(s).replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));
}

function utf8Bytes(s) {
  return new TextEncoder().encode(s);
}

function nowRFC3339Nanoish() {
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

// ===== Network Helpers =====
async function sendNotarizationRequest(payload) {
  const res = await fetch(ADD_URL, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      "accept": "application/json",
    },
    body: payload,
  });

  const raw = await res.text();
  if (!res.ok) throw new Error(`Server ${res.status}: ${raw}`);

  const ct = (res.headers.get("content-type") || "").toLowerCase();
  const isJson = ct.includes("application/json");

  return { raw, isJson };
}

// ===== JSON & Crypto Logic =====
async function sha256HexOfFile(file) {
  const ab = await file.arrayBuffer();
  const digest = await crypto.subtle.digest("SHA-256", ab);
  return hexFromBytes(digest);
}

// ===== Business Logic =====
function buildEventObject(data, payloadHex) {
  requireNonEmpty("issuer.entity_id", data.issuerId);
  requireNonEmpty("doc_uid", data.docUid);
  requireNonEmpty("title", data.title);

  if (!Number.isFinite(data.docVersion)) {
    throw new Error("doc_version non finito non ammesso");
  }
  if (!Number.isInteger(data.docVersion)) {
    throw new Error("doc_version deve essere un intero");
  }
  if (!Number.isSafeInteger(data.docVersion)) {
    throw new Error("doc_version fuori range JS (non safe integer)");
  }
  if (data.docVersion < 1) {
    throw new Error("doc_version deve essere un intero >= 1");
  }

  const needsPrev = data.eventType === "UPDATE" || data.eventType === "REVOKE" || data.eventType === "EXPIRE";
  
  if (needsPrev) {
    requireNonEmpty("prev_event_id", data.prevEventId);
    if (!isUUIDv4(data.prevEventId)) throw new Error("prev_event_id deve essere UUIDv4");
  } else if (data.prevEventId) {
    throw new Error("prev_event_id non deve essere presente per CREATE");
  }

  const needsPayload = data.eventType === "CREATE" || data.eventType === "UPDATE";
  const payloadHash = needsPayload ? { alg: "sha-256", value: `hex:${payloadHex}` } : undefined;

  if (needsPayload && (!payloadHex || payloadHex.length !== 64)) {
    throw new Error("payload_hash sha-256 hex non valido");
  }

  if (data.eventType === "CREATE" && data.docVersion !== 1) {
    throw new Error("CREATE richiede doc_version=1");
  }
  if (data.eventType === "UPDATE" && data.docVersion < 2) {
    throw new Error("UPDATE richiede doc_version>=2");
  }

  return {
    schema: "pa-notary-event@1",
    event_type: data.eventType,
    doc_uid: data.docUid,
    doc_version: data.docVersion,
    ...(needsPrev ? { prev_event_id: data.prevEventId } : {}),
    ...(payloadHash ? { payload_hash: payloadHash } : {}),
    issuer: {
      entity_id: data.issuerId,
      ...(data.issuerName ? { name: data.issuerName } : {}),
    },
    issued_at: nowRFC3339Nanoish(),
    title: data.title,
    ...(data.description ? { description: data.description } : {}),
    ...(data.notes ? { notes: data.notes } : {}),
  };
}

populateIssuerOptions();
syncIssuerName();
issuerIdEl.addEventListener("change", syncIssuerName);

// ===== UI Actions =====
btnBuild.addEventListener("click", async () => {
  try {
    respEl.textContent = "(vuoto)";
    setStatus("Elaborazione...", "ok");
    lastNotarizedJSON = null;
    btnDownload.disabled = true;

    const file = fileEl.files?.[0];
    if (!file) throw new Error("Seleziona un file");

    const formData = getFormData();
    const payloadHex = await sha256HexOfFile(file);
    const ev = buildEventObject(formData, payloadHex);

    const reqJSON = JSON.stringify(ev);
    const size = utf8Bytes(reqJSON).length;
    if (size > MAX_JSON_BYTES) throw new Error(`JSON troppo grande: ${size} bytes (max ${MAX_JSON_BYTES})`);

    lastRequestJSON = reqJSON;
    outEl.textContent = reqJSON;

    btnSend.disabled = false;
    setStatus(`OK. JSON richiesta pronto (${size} bytes).`, "ok");
  } catch (e) {
    lastRequestJSON = null;
    lastNotarizedJSON = null;
    outEl.textContent = "(vuoto)";
    btnSend.disabled = true;
    btnDownload.disabled = true;
    setStatus(e?.message || String(e), "err");
  }
});

btnSend.addEventListener("click", async () => {
  try {
    if (!lastRequestJSON) throw new Error("Prima genera il JSON");
    respEl.textContent = "(inviando...)";

    const { raw, isJson } = await sendNotarizationRequest(lastRequestJSON);

    if (isJson) {
      const parsed = JSON.parse(raw);
      respEl.textContent = JSON.stringify(parsed, null, 2);
      
      if (parsed?.notarized_json === undefined) {
        throw new Error("Risposta /add senza notarized_json");
      }
      
      lastNotarizedJSON = typeof parsed.notarized_json === "string"
        ? parsed.notarized_json
        : JSON.stringify(parsed.notarized_json);
      
      btnDownload.disabled = false;

      const idx = parsed?.log_index;
      if (idx !== undefined && idx !== null) {
        setStatus(`Notarizzato con successo. Log index: ${idx}`, "ok");
      } else {
        setStatus("Notarizzato con successo.", "ok");
      }
    } else {
      lastNotarizedJSON = null;
      btnDownload.disabled = true;
      respEl.textContent = raw.trim();
      setStatus("Notarizzato con successo.", "ok");
    }
  } catch (e) {
    lastNotarizedJSON = null;
    btnDownload.disabled = true;
    respEl.textContent = "(errore)";
    setStatus(e?.message || String(e), "err");
  }
});

btnDownload.addEventListener("click", () => {
  try {
    if (!lastNotarizedJSON) throw new Error("Prima notarizza il documento con /add");
    downloadText("event_notarized.json", lastNotarizedJSON, "application/json");
  } catch (e) {
    setStatus(e?.message || String(e), "err");
  }
});

document.getElementById("btnMock").addEventListener("click", () => {
  const mockIssuers = DEMO_ISSUERS.filter((issuer) => issuer.entityId);
  const tipiDoc = ["Determina", "Ordinanza", "Delibera", "Protocollo", "Certificato", "Circolare"];
  const possibiliNote = [
    "Generato automaticamente per test notarizzazione.",
    "Documento verificato dall'ufficio legale.",
    "Contiene allegati tecnici riservati.",
    "Notarizzazione prioritaria.",
    "In attesa di firma digitale aggiuntiva.",
    "Copia conforme all'originale cartaceo.",
    "",
  ];

  const issuer = mockIssuers[Math.floor(Math.random() * mockIssuers.length)];
  const tipo = tipiDoc[Math.floor(Math.random() * tipiDoc.length)];
  const nota = possibiliNote[Math.floor(Math.random() * possibiliNote.length)];
  const anno = 2026;
  const num = Math.floor(Math.random() * 100000);

  issuerIdEl.value = issuer.entityId;
  syncIssuerName();
  docUidEl.value = `${tipo.toUpperCase()}/${anno}/${num}`;

  eventTypeEl.value = "CREATE";
  docVersionEl.value = 1;
  prevEventIdEl.value = "";

  titleEl.value = `${tipo} dirigenziale n. ${num}`;
  descriptionEl.value = `Documento relativo alla gestione pratica ${num} per ${issuer.name}.`;
  notesEl.value = nota;
});
