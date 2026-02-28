const DEBUG = false;
const DPI_THR_BYTES = 64 * 1024;
const MAX_URI_X_SIZE = 7 * 1024;
const RIPE_API_URL = "https://stat.ripe.net/data/";

const ALIVE_KEY = "alive";
const ALIVE_NO = 0;
const ALIVE_YES = 1;
const ALIVE_UNKNOWN = 2;

const DPI_METHOD_KEY = "dpi";
const DPI_METHOD_NOT_DETECTED = 0;
const DPI_METHOD_DETECTED = 1;
const DPI_METHOD_PROBABLY = 2;
const DPI_METHOD_POSSIBLE = 3;
const DPI_METHOD_UNLIKELY = 4;

let testSuite = []; // Fetched from ./suite.v2.json
let timeoutMs = 15000;
let clientAsn = 0;
let resultItems = {};

const getParamsHandler = () => {
  const params = new URLSearchParams(window.location.search);

  const host = params.get("host");
  if (host) {
    const provider = params.get("provider") || "Custom";
    const newTest = { id: `CUSTOM-01`, provider, host };
    testSuite.push(newTest);
  }

  timeoutMs = parseInt(params.get("timeout")) || timeoutMs;
};

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

const headerEl = document.getElementById("header");
const startButtonEl = document.getElementById("start-btn");
const shareButtonEl = document.getElementById("share-btn");
const statusEl = document.getElementById("status");
const logEl = document.getElementById("log");
const resultsEl = document.getElementById("results");
const shareTsEl = document.getElementById("shareTs");
const asnEl = document.getElementById("asn");

const toggleUI = (locked) => {
  shareButtonEl.disabled = locked;
  startButtonEl.disabled = locked;
  startButtonEl.textContent = locked ? "🔍 ..." : "🔍 Start";
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

  resultItems = {};

  try {
    const tasks = [];
    for (let t of testSuite) {
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
    const dpiTimeoutId = setTimeout(() => dpiCtrl.abort(), timeoutMs);
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
    const dpiTimeoutId = setTimeout(() => dpiCtrl.abort(), timeoutMs);
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
  resultItems[id] = {};
  setPrettyProvider(providerCell, provider, country);
  setStatus(aliveStatusCell, "Checking ⏰", "");
  setStatus(dpiStatusCell, "Waiting ⏰", "");

  try {
    // alive check
    const aliveCtrl = new AbortController();
    const aliveTimeoutId = setTimeout(() => aliveCtrl.abort(), timeoutMs);
    const url = `https://${host}/`
    await fetch(getUniqueUrl(url), getDefaultFetchOpt(aliveCtrl, "HEAD"));
    clearTimeout(aliveTimeoutId);
    logPush("INFO", prefix, `alived: yes 🟢, reqtime: ${timeElapsed(t0)}`);
    resultItems[id][ALIVE_KEY] = ALIVE_YES;
    alive = true;
    possibleAlive = true;
  }
  catch (e) {
    console.log(e);
    if (e.name === "AbortError") {
      logPush("INFO", prefix, `alived: no 🔴, reqtime: ${timeElapsed(t0)}`);
      resultItems[id][ALIVE_KEY] = ALIVE_NO;
    } else {
      logPush("INFO", prefix, `alived: unknown ⚠️, reqtime: ${timeElapsed(t0)}`);
      resultItems[id][ALIVE_KEY] = ALIVE_UNKNOWN;
      possibleAlive = true;
    }
  }

  setPrettyAlive(aliveStatusCell, resultItems[id][ALIVE_KEY]);
  if (!alive && !possibleAlive) {
    setPrettyDpi(dpiStatusCell, ALIVE_NO, null); // -> skip
    resultItems[id][DPI_METHOD_KEY] = DPI_METHOD_NOT_DETECTED; // default value
    return;
  }

  // dpi check
  setStatus(dpiStatusCell, "Checking ⏰", "");
  const m1 = await dpiHugeBodyPostMethod(alive, host);
  if (m1 == DPI_METHOD_DETECTED) {
    logPush("INFO", prefix, `tcp 16-20: detected❗️, method: 1`);
    setPrettyDpi(dpiStatusCell, resultItems[id][ALIVE_KEY], m1);
    resultItems[id][DPI_METHOD_KEY] = DPI_METHOD_DETECTED;
    return;
  }

  t0 = performance.now();
  const m2 = await dpiHugeReqlineHeadMethod(alive, host);
  resultItems[id][DPI_METHOD_KEY] = m2;
  setPrettyDpi(dpiStatusCell, resultItems[id][ALIVE_KEY], m2);

  const logDpiMap = {
    [DPI_METHOD_DETECTED]: `tcp 16-20: detected❗️, method: 2`,
    [DPI_METHOD_PROBABLY]: `tcp 16-20: probably detected ⚠️, reqtime: ${timeElapsed(t0)}`,
    [DPI_METHOD_POSSIBLE]: `tcp 16-20: possible detected ⚠️, reqtime: ${timeElapsed(t0)}`,
    [DPI_METHOD_UNLIKELY]: `tcp 16-20: unlikely ⚠️, reqtime: ${timeElapsed(t0)}`,
    [DPI_METHOD_NOT_DETECTED]: `tcp 16-20: not detected ✅, reqtime: ${timeElapsed(t0)}`,
  }

  logPush("INFO", prefix, logDpiMap[m2]);
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

const fetchAsnBasic = async (asn) => {
  const holder = (await (await fetch(RIPE_API_URL + "as-overview/data.json?resource=" + asn)).json()).data.holder;
  asnEl.innerHTML = `ASN: <a href="https://bgp.he.net/AS${asn}" target="_blank">AS${asn}</a> (<i>${holder}</i>)`;
};

const fetchAsn = async () => {
  try {
    const ip = (await (await fetch(RIPE_API_URL + "whats-my-ip/data.json")).json()).data.ip;
    const asn = (await (await fetch(RIPE_API_URL + "prefix-overview/data.json?resource=" + ip)).json()).data.asns[0];
    clientAsn = Number(asn.asn);
    const geo = (await (await fetch(RIPE_API_URL + "maxmind-geo-lite/data.json?resource=" + ip)).json()).data.located_resources[0].locations[0];
    asnEl.innerHTML = `ASN: <a href="https://bgp.he.net/AS${asn.asn}" target="_blank">AS${asn.asn}</a> (<i>${asn.holder}</i>)<span class="asn-br"></span>${geo.country}, ${geo.city || "—"}`;
  } catch (err) {
    console.error("Fetch ASN err:", err);
  }
};

const fetchSuite = async () => {
  try {
    testSuite = await (await fetch(getUniqueUrl("./suite.v2.json"))).json();
    startButtonEl.disabled = false;
  } catch {
    logPush("ERR", null, `Fetch suite failed. Probably a CORS issue (running locally?).`);
  }
};

const prettyTs = (ts) => {
  return ts.toISOString().slice(0, 16).replace('T', ' ');
}

const setPrettyProvider = (el, provider, country) => {
  el.textContent = `${country} ${provider}`;
};

const setPrettyDpi = (el, alive, dpi) => {
  if (alive == ALIVE_NO) {
    setStatus(el, "Skip ⚠️", "skip");
    return;
  }
  const m = {
    [DPI_METHOD_NOT_DETECTED]: () => setStatus(el, "No ✅", "ok"),
    [DPI_METHOD_DETECTED]: () => setStatus(el, "Detected❗️", "bad"),
    [DPI_METHOD_PROBABLY]: () => setStatus(el, "Probably❗️", "skip"),
    [DPI_METHOD_POSSIBLE]: () => setStatus(el, "Possible ⚠️", "skip"),
    [DPI_METHOD_UNLIKELY]: () => setStatus(el, "Unlikely ⚠️", "skip"),
  };
  m[dpi]();
};

const setPrettyAlive = (el, alive) => {
  const m = {
    [ALIVE_NO]: () => setStatus(el, "No 🔴", "bad"),
    [ALIVE_YES]: () => setStatus(el, "Yes 🟢", "ok"),
    [ALIVE_UNKNOWN]: () => setStatus(el, "Unknown ⚠️", "skip"),
  }
  m[alive]();
};

const renderShare = (share) => {
  shareTsEl.textContent = `Test timestamp: ${prettyTs(share.ts)}`;
  for (let v of share.items) {
    const row = resultsEl.insertRow();
    const idCell = row.insertCell();
    const providerCell = row.insertCell();
    const aliveStatusCell = row.insertCell();
    const dpiStatusCell = row.insertCell();

    idCell.textContent = v.id;
    setPrettyProvider(providerCell, v.provider, v.country);
    setPrettyAlive(aliveStatusCell, v.alive);
    setPrettyDpi(dpiStatusCell, v.alive, v.dpi);
  }
};

// the contract should not be changed because it is used by historical functions
const rawImport = async (url) => {
  const res = await fetch(url);
  const code = await res.text();
  const blobUrl = URL.createObjectURL(
    new Blob([code], { type: 'text/javascript' })
  );
  return await import(blobUrl);
};

const tryHandleShare = async () => {
  const params = new URLSearchParams(window.location.search);
  const share = params.get("share");
  if (share) {
    const link = location.pathname;
    headerEl.innerHTML = `Want to try it too? Click <a href="${link}">here</a> ⚡`;
    headerEl.hidden = false;

    try {
      resultsEl.hidden = true;
      logEl.hidden = true;
      const buf = Uint8Array.fromBase64(share, { alphabet: "base64url" });
      const h = await import('./share/helpers.js');
      const commitHex = h.getCommitHex(buf);
      const relPath = "share/decoder.js";
      let decoderUrl = `https://raw.githubusercontent.com/${h.REPO}/${commitHex}/ru/tcp-16-20/${relPath}`;
      if (DEBUG) {
        decoderUrl = "./" + relPath;
      }

      const { decodeShare } = await rawImport(decoderUrl);
      const decoded = await decodeShare(h.REPO, commitHex, buf);
      fetchAsnBasic(decoded.asn);
      renderShare(decoded);
      resultsEl.hidden = false;
    }
    catch (e) {
      console.log(e);
      shareTsEl.hidden = true;
      asnEl.hidden = true;

      if (typeof Uint8Array.prototype.fromBase64 !== "function") {
        alert("To see the results, you need to update your browser.");
        return true;
      }
      alert("The results are out of date or internal error.");
    }
    return true;
  }
  return false;
};

startButtonEl.onclick = () => {
  logEl.textContent = "";
  toggleUI(true);
  localStorage.clear();
  sessionStorage.clear();
  startOrchestrator();
};

shareButtonEl.onclick = async () => {
  const prevContent = shareButtonEl.textContent;
  shareButtonEl.textContent = "🔗 ..."
  shareButtonEl.disabled = true;

  try {
    const encoded = await encodeShare(clientAsn, resultItems);
    const url = `${window.location.origin + window.location.pathname}?share=${encoded}`;
    try {
      await navigator.clipboard.writeText(url);
      alert("Link to results copied to clipboard.");
    } catch {
      alert("Error writing to clipboard. Permissions granted?");
    }
  }
  catch {
    if (typeof Uint8Array.prototype.toBase64 !== "function") {
      alert("To share the results, you should update your browser.");
      return true;
    }

    alert("Error when encoding results.");
  }

  shareButtonEl.textContent = prevContent;
  shareButtonEl.disabled = false;
};

document.addEventListener("DOMContentLoaded", async () => {
  if (DEBUG) {
    console.log("debug mode: on");
    insertDebugRow();
  }

  if (await tryHandleShare()) {
    return;
  }

  getParamsHandler();
  fetchAsn();
  await fetchSuite();
});
