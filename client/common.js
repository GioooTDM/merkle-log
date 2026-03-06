export const API_BASE = "http://localhost:2025";

export const DEMO_ISSUERS = [
  { entityId: "", name: "Seleziona ente..." },
  { entityId: "IPA:COMUNE-XYZ", name: "Comune di Esempio - Ufficio Protocollo" },
  { entityId: "IPA:C_ROMA", name: "Comune di Roma" },
  { entityId: "IPA:C_MILANO", name: "Comune di Milano" },
  { entityId: "IPA:C_TORINO", name: "Comune di Torino" },
  { entityId: "IPA:C_NAPOLI", name: "Comune di Napoli" },
  { entityId: "IPA:C_BOLOGNA", name: "Comune di Bologna" },
  { entityId: "IPA:C_FIRENZE", name: "Comune di Firenze" },
  { entityId: "IPA:R_LAZIO", name: "Regione Lazio" },
  { entityId: "IPA:R_LOMBARDIA", name: "Regione Lombardia" },
  { entityId: "IPA:R_PIEMONTE", name: "Regione Piemonte" },
  { entityId: "IPA:R_SICILIA", name: "Regione Siciliana" },
  { entityId: "IPA:M_INTERNO", name: "Ministero dell'Interno" },
  { entityId: "IPA:M_GIUSTIZIA", name: "Ministero della Giustizia" },
  { entityId: "IPA:M_ECONOMIA", name: "Ministero dell'Economia e delle Finanze" },
  { entityId: "IPA:INPS", name: "INPS" },
];

export function el(id) {
  return document.getElementById(id);
}

export function safeStr(value, fallback = "-") {
  return value === undefined || value === null || value === "" ? fallback : String(value);
}

export function normalizeHex(value) {
  return String(value || "").trim().toLowerCase().replace(/^hex:/, "");
}

export function formatDate(value) {
  if (!value) return "-";

  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return String(value);

  const pad = (n) => String(n).padStart(2, "0");
  const year = date.getFullYear();
  const month = pad(date.getMonth() + 1);
  const day = pad(date.getDate());
  const hours = pad(date.getHours());
  const mins = pad(date.getMinutes());
  const secs = pad(date.getSeconds());

  return `${year}-${month}-${day} ${hours}:${mins}:${secs}`;
}

export async function fetchText(url, options) {
  const res = await fetch(url, options);
  const text = await res.text();
  if (!res.ok) {
    throw new Error(`HTTP ${res.status}: ${text || "Richiesta fallita"}`);
  }
  return text;
}

export async function fetchJson(url, options) {
  const text = await fetchText(url, options);
  return JSON.parse(text);
}

export function downloadBlob(filename, blob) {
  const link = document.createElement("a");
  link.href = URL.createObjectURL(blob);
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(link.href);
}

export function downloadText(filename, text, mimeType = "text/plain;charset=utf-8") {
  downloadBlob(filename, new Blob([text], { type: mimeType }));
}

export function downloadJson(filename, data) {
  downloadText(filename, JSON.stringify(data, null, 2), "application/json");
}

export function parseCheckpointSize(raw) {
  const lines = String(raw).split(/\r?\n/).map((s) => s.trim()).filter(Boolean);
  const sizeLine = lines.find((line) => /^\d+$/.test(line));
  if (!sizeLine) throw new Error("Formato checkpoint non riconosciuto");
  return Number(sizeLine);
}
