const BIN_THR_BYTES = 24 * 1024;

// Where possible, we should use stable endpoints (with the same response size every time, with a long-lived URL, without versions, etc).
// The `thresholdBytes` is evaluated by utils/http_compression_prober.py (in the repo) and should be set to the max value from all compressors.
// This way, we ensure that more incoming data than the "tcp 16-20" limit has passed through the network.
let TEST_SUITE = []; // Fetched from ./suite.json
let TIMEOUT_MS = 5000;

(function getParamsHandler() {
  const params = new URLSearchParams(window.location.search);

  const url = params.get("url");
  if (url) {
    const provider = params.get("provider") || "Custom";
    const times = parseInt(params.get("times")) || 1;
    const thresholdBytes = parseInt(params.get("thrBytes")) || BIN_THR_BYTES;
    const newTest = { id: `CUST-01`, provider, times, url, thresholdBytes };
    TEST_SUITE.push(newTest);
  }

  TIMEOUT_MS = parseInt(params.get("timeout")) || TIMEOUT_MS;
})();

const fetchOpt = ctrl => ({
  method: "GET",
  credentials: "omit",
  cache: "no-store",
  signal: ctrl.signal,
  redirect: "manual",
  keepalive: true
});

const startButton = document.getElementById("start");
const status = document.getElementById("status");
const log = document.getElementById("log");
const results = document.getElementById("results");

const httpCodes = {};

const toggleUI = (locked) => {
  startButton.disabled = locked;
  startButton.textContent = locked ? "..." : "Start";
  status.className = locked ? "status-checking" : "status-ready";
};

const setStatus = (col, text, cls) => {
  col.textContent = text;
  col.className = cls;
  if (cls === "bad") status.className = "status-error";
};

const logPush = (level, prefix, msg) => {
  const now = new Date();
  const ts = now.toLocaleTimeString([], { hour12: false }) + "." + now.getMilliseconds().toString().padStart(3, "0");
  log.textContent += `[${ts}] ${prefix ? prefix + "/" : ""}${level}: ${msg}\n`;
  log.scrollTop = log.scrollHeight;
};

const timeElapsed = t0 => `${(performance.now() - t0).toFixed(1)} ms`;
const getHttpStatus = id => httpCodes[id];

const getUniqueUrl = url => {
  return url.includes('?') ? `${url}&t=${Math.random()}` : `${url}?t=${Math.random()}`;
};

const startOrchestrator = async () => {
  status.textContent = "Checking ⏰";
  status.className = "status-checking";
  for (let i = results.rows.length - 1; i > 0; i--) {
    results.deleteRow(i);
  }

  try {
    const tasks = [];
    for (let t of TEST_SUITE) {
      for (let i = 0; i < t.times; i++) {
        tasks.push(checkDpi(t.times > 1 ? `${t.id}@${i}` : t.id, t.provider, t.url, t.thresholdBytes, t.country));
      }
    }

    await Promise.all(tasks);
    status.textContent = "Ready ⚡";
    status.className = "status-ready";
  } catch (e) {
    status.textContent = "Unexpected error ⚠️";
    logPush("ERR", null, `Unexpected error => ${e}`);
    status.className = "status-error";
  }
  logPush("INFO", null, "Done.");
  toggleUI(false);
};

const checkDpi = async (id, provider, url, thresholdBytes, country) => {
  const prefix = `DPI checking(#${id})`;
  const t0 = performance.now();
  const ctrl = new AbortController();
  const timeoutId = setTimeout(() => ctrl.abort(), TIMEOUT_MS);

  const row = results.insertRow();
  const numCell = row.insertCell();
  const providerCell = row.insertCell();
  const statusCell = row.insertCell();

  numCell.textContent = id;
  providerCell.textContent = `${country} ${provider}`;
  setStatus(statusCell, "Checking ⏰", "");

  try {
    const r = await fetch(getUniqueUrl(url), fetchOpt(ctrl));
    logPush("INFO", prefix, `HTTP ${r.status}`);
    httpCodes[id] = r.status;
    const reader = r.body.getReader();
    let received = 0, ok = false;

    while (true) {
      const { done, value } = await reader.read();
      if (done) {
        clearTimeout(timeoutId);
        logPush("INFO", prefix, `Stream complete without timeout (${timeElapsed(t0)})`);
        if (!ok) {
          logPush("WARN", prefix, `Stream ended but data is too small`);
          setStatus(statusCell, "Possibly detected ⚠️", "");
        }
        break;
      }

      received += value.byteLength;
      logPush("INFO", prefix, `Received chunk: ${value.byteLength} bytes, total: ${received}`);

      if (!ok && received >= thresholdBytes) {
        clearTimeout(timeoutId);
        await reader.cancel();
        ok = true;
        logPush("INFO", prefix, `Early complete (${timeElapsed(t0)})`);
        setStatus(statusCell, "Not detected ✅", "ok");
        break;
      }
    }
  } catch (e) {
    clearTimeout(timeoutId);
    if (e.name === "AbortError") {
      const statusCode = getHttpStatus(id);
      let reason = statusCode ? "READ" : "CONN";
      logPush("ERR", prefix, `${reason} timeout reached (${timeElapsed(t0)})`);
      setStatus(statusCell, statusCode ? "Detected❗️" : "Conn timeout❗️", "bad");
    } else {
      logPush("ERR", prefix, `Fetch/read error => ${e}`);
      setStatus(statusCell, "Failed to complete detection ⚠️", "");
    }
  }
};

startButton.onclick = () => {
  log.textContent = "";
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
    TEST_SUITE = await (await fetch(getUniqueUrl("./suite.json"))).json();
    startButton.disabled = false;
  } catch {
    logPush("ERR", null, `Fetch suite failed. Probably a CORS issue (running locally?).`);
  }
};

document.addEventListener("DOMContentLoaded", async () => {
  await fetchSuite();
  await fetchAsn();
});
