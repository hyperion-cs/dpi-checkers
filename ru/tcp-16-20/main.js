const DEBUG = false;
const DPI_THR_BYTES = 64 * 1024;
const MAX_URI_X_SIZE = 7 * 1024;

const DPI_METHOD_NOT_DETECTED = 0;
const DPI_METHOD_DETECTED = 1;
const DPI_METHOD_PROBABLY = 2;
const DPI_METHOD_POSSIBLE = 3;
const DPI_METHOD_UNLIKELY = 4;

let TEST_SUITE = []; // Fetched from ./suite.v2.json
let TIMEOUT_MS = 15000;

(function getParamsHandler() {
  const params = new URLSearchParams(window.location.search);

  const host = params.get("host");
  if (host) {
    const provider = params.get("provider") || "Custom";
    const newTest = { id: `CUSTOM-01`, provider, host };
    TEST_SUITE.push(newTest);
  }

  TIMEOUT_MS = parseInt(params.get("timeout")) || TIMEOUT_MS;
})();

const getDefaultFetchOpt = (ctrl, method = "GET",) => ({
  method,
  mode: "no-cors",
  referrer: "",
  credentials: "omit",
  cache: "no-store",
  signal: ctrl.signal,
  redirect: "follow",
  // The body size for keepalive requests is limited to 64 kibibytes.
  // https://developer.mozilla.org/en-US/docs/Web/API/RequestInit#keepalive
  keepalive: false
});

const startButtonEl = document.getElementById("start");
const statusEl = document.getElementById("status");
const logEl = document.getElementById("log");
const resultsEl = document.getElementById("results");

const toggleUI = (locked) => {
  startButtonEl.disabled = locked;
  startButtonEl.textContent = locked ? "..." : "Start";
  statusEl.className = locked ? "status-checking" : "status-ready";
};

const setStatus = (col, text, cls) => {
  col.textContent = text;
  col.className = cls;
  if (cls === "bad") statusEl.className = "status-error";
};

const logPush = (level, prefix, msg) => {
  const now = new Date();
  const ts = now.toLocaleTimeString([], { hour12: false }) + "." + now.getMilliseconds().toString().padStart(3, "0");
  logEl.textContent += `[${ts}] ${prefix ? prefix + "/" : ""}${level}: ${msg}\n`;
  logEl.scrollTop = logEl.scrollHeight;
};

const timeElapsed = t0 => `${(performance.now() - t0).toFixed(1)} ms`;
const getHttpStatus = id => httpCodes[id];

const getUniqueUrl = url => {
  return url.includes('?') ? `${url}&t=${Math.random()}` : `${url}?t=${Math.random()}`;
};

const getRandomData = size => {
  const data = new Uint8Array(size);
  const grvMax = 64 * 1024; // https://developer.mozilla.org/en-US/docs/Web/API/Crypto/getRandomValues
  for (let offset = 0; offset < size; offset += grvMax) {
    crypto.getRandomValues(data.subarray(offset, offset + grvMax));
  }
  return data;
};

const getRandomSafeData = (n) => {
  const chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz";
  return Array.from({ length: n }, () =>
    chars[Math.floor(Math.random() * chars.length)]
  ).join("");
};

const startOrchestrator = async () => {
  statusEl.textContent = "Checking ⏰";
  statusEl.className = "status-checking";

  for (let i = resultsEl.rows.length - 1; i > 0; i--) {
    resultsEl.deleteRow(i);
  }

  try {
    const tasks = [];
    for (let t of TEST_SUITE) {
      tasks.push(checkDpi(t.id, t.provider, t.host, t.country));
    }

    await Promise.all(tasks);
    statusEl.textContent = "Ready ⚡";
    statusEl.className = "status-ready";
  } catch (e) {
    statusEl.textContent = "Unexpected error ⚠️";
    logPush("ERR", null, `Unexpected error => ${e}`);
    statusEl.className = "status-error";
  }
  logPush("INFO", null, "Done.");
  toggleUI(false);
};

const handleDpiMethodErr = (alive, e) => {
  if (e.name === "AbortError") {
    if (alive) {
      return DPI_METHOD_DETECTED; // alive — ok, push — timeout
    }
    return DPI_METHOD_PROBABLY; // alive — instant error, push — timeout
  }
  if (alive) {
    return DPI_METHOD_POSSIBLE; // alive — ok, push — instant error
  }
  return DPI_METHOD_UNLIKELY; // alive — instant error, push — instant error
};

const dpiHugeBodyPostMethod = async (alive, host) => {
  try {
    const dpiCtrl = new AbortController();
    const dpiTimeoutId = setTimeout(() => dpiCtrl.abort(), TIMEOUT_MS);
    const opt = getDefaultFetchOpt(dpiCtrl, "POST")
    opt.body = getRandomData(DPI_THR_BYTES)
    const url = `https://${host}/`;
    await fetch(getUniqueUrl(url), opt);
    clearTimeout(dpiTimeoutId);
  } catch (e) {
    return handleDpiMethodErr(alive, e);
  }

  return DPI_METHOD_NOT_DETECTED;
};

const dpiHugeReqlineHeadMethod = async (alive, host) => {
  try {
    const times = DPI_THR_BYTES / MAX_URI_X_SIZE;
    const dpiCtrl = new AbortController();
    const dpiTimeoutId = setTimeout(() => dpiCtrl.abort(), TIMEOUT_MS);
    for (let i = 0; i < times; i++) {
      const opt = getDefaultFetchOpt(dpiCtrl, "HEAD") // HEAD seems to be stable keep-alived 
      const url = `https://${host}/?x=${getRandomSafeData(MAX_URI_X_SIZE)}`
      await fetch(getUniqueUrl(url), opt);
    }
    clearTimeout(dpiTimeoutId);
  } catch (e) {
    return handleDpiMethodErr(alive, e);
  }

  return DPI_METHOD_NOT_DETECTED;
};

const checkDpi = async (id, provider, host, country) => {
  const prefix = `DPI checking(#${id})`;
  let t0 = performance.now();

  const row = resultsEl.insertRow();
  const idCell = row.insertCell();
  const providerCell = row.insertCell();
  const aliveStatusCell = row.insertCell();
  const dpiStatusCell = row.insertCell();

  let alive = false;
  let possibleAlive = false;

  idCell.textContent = id;
  providerCell.textContent = `${country} ${provider}`;
  setStatus(aliveStatusCell, "Checking ⏰", "");
  setStatus(dpiStatusCell, "Waiting ⏰", "");

  try {
    // alive check
    const aliveCtrl = new AbortController();
    const aliveTimeoutId = setTimeout(() => aliveCtrl.abort(), TIMEOUT_MS);
    const url = `https://${host}/`
    await fetch(getUniqueUrl(url), getDefaultFetchOpt(aliveCtrl, "HEAD"));
    clearTimeout(aliveTimeoutId);
    logPush("INFO", prefix, `alived: yes 🟢, reqtime: ${timeElapsed(t0)}`);
    setStatus(aliveStatusCell, "Yes 🟢", "ok");
    alive = true;
    possibleAlive = true;
  }
  catch (e) {
    console.log(e);
    if (e.name === "AbortError") {
      logPush("INFO", prefix, `alived: no 🔴, reqtime: ${timeElapsed(t0)}`);
      setStatus(aliveStatusCell, "No 🔴", "bad");
    } else {
      logPush("INFO", prefix, `alived: unknown ⚠️, reqtime: ${timeElapsed(t0)}`);
      setStatus(aliveStatusCell, "Unknown ⚠️", "skip");
      possibleAlive = true;
    }
  }

  if (!alive && !possibleAlive) {
    setStatus(dpiStatusCell, "Skip ⚠️", "skip");
    return;
  }

  // dpi check
  setStatus(dpiStatusCell, "Checking ⏰", "");
  const m1 = await dpiHugeBodyPostMethod(alive, host);
  if (m1 == DPI_METHOD_DETECTED) {
    logPush("INFO", prefix, `tcp 16-20: detected❗️, method: 1`);
    setStatus(dpiStatusCell, "Detected❗️", "bad");
    return;
  }

  t0 = performance.now();
  const m2 = await dpiHugeReqlineHeadMethod(alive, host);
  if (m2 == DPI_METHOD_DETECTED) {
    logPush("INFO", prefix, `tcp 16-20: detected❗️, method: 2`);
    setStatus(dpiStatusCell, "Detected❗️", "bad");
    return;
  }

  if (m2 == DPI_METHOD_PROBABLY) {
    logPush("INFO", prefix, `tcp 16-20: probably detected ⚠️, reqtime: ${timeElapsed(t0)}`);
    setStatus(dpiStatusCell, "Probably ❗️", "skip");
    return;
  }

  if (m2 == DPI_METHOD_POSSIBLE) {
    logPush("INFO", prefix, `tcp 16-20: possible detected ⚠️, reqtime: ${timeElapsed(t0)}`);
    setStatus(dpiStatusCell, "Possible ⚠️", "skip");
    return;
  }

  if (m2 == DPI_METHOD_UNLIKELY) {
    logPush("INFO", prefix, `tcp 16-20: unlikely ⚠️, reqtime: ${timeElapsed(t0)}`);
    setStatus(dpiStatusCell, "Unlikely ⚠️", "skip");
    return;
  }

  logPush("INFO", prefix, `tcp 16-20: not detected ✅, reqtime: ${timeElapsed(t0)}`);
  setStatus(dpiStatusCell, "No ✅", "ok");
};

const insertDebugRow = () => {
  const row = resultsEl.insertRow();
  const idCell = row.insertCell();
  const providerCell = row.insertCell();
  const aliveStatusCell = row.insertCell();
  const dpiStatusCell = row.insertCell();

  idCell.textContent = "XY.ABCD-01"
  providerCell.textContent = "🇺🇸 AbcdefQwerty"
  aliveStatusCell.textContent = "Checking ⏰"
  dpiStatusCell.textContent = "Checking ⏰"
}

startButtonEl.onclick = () => {
  logEl.textContent = "";
  toggleUI(true);
  localStorage.clear();
  sessionStorage.clear();
  startOrchestrator();
};

const fetchAsn = async () => {
  try {
    const RIPE_API_URL = "https://stat.ripe.net/data/";
    const ip = (await (await fetch(RIPE_API_URL + "whats-my-ip/data.json")).json()).data.ip;
    const asn = (await (await fetch(RIPE_API_URL + "prefix-overview/data.json?resource=" + ip)).json()).data.asns[0];
    const geo = (await (await fetch(RIPE_API_URL + "maxmind-geo-lite/data.json?resource=" + ip)).json()).data.located_resources[0].locations[0];
    const el = document.getElementById("asn");
    el.innerHTML = `ASN: <a href="https://bgp.he.net/AS${asn.asn}" target="_blank">AS${asn.asn}</a> (<i>${asn.holder}</i>)<span class="asn-br"></span>${geo.country}, ${geo.city || "—"}`;
  } catch (err) {
    console.error("Fetch ASN err:", err);
  }
};

const fetchSuite = async () => {
  try {
    TEST_SUITE = await (await fetch(getUniqueUrl("./suite.v2.json"))).json();
    startButtonEl.disabled = false;
  } catch {
    logPush("ERR", null, `Fetch suite failed. Probably a CORS issue (running locally?).`);
  }
};

document.addEventListener("DOMContentLoaded", async () => {
  if (DEBUG) {
    console.log("debug mode: on")
    insertDebugRow();
  }

  await fetchSuite();
  await fetchAsn();
});
