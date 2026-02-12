const TEST_SUITE = [
  { "name": "Yandex", "asns": ["13238", "44534", "200350", "202611", "208398", "208795", "210656", "212066", "215013", "215109"] },
  { "name": "VK", "asns": ["28709", "47541", "47542", "47764", "60863", "62243", "199295", "207581"] },
  { "name": "EdgeCenter", "asns": ["201589", "207059", "210756"] },
];

let TIMEOUT_MS = 5000;
let SUBNET_SAMPLE_SIZE = 25;
let SUBNET_ALIVE_MIN = 3;
let SUBNET_ONLY_24_PREFIX = true;

(function getParamsHandler() {
  const params = new URLSearchParams(window.location.search);

  TIMEOUT_MS = parseInt(params.get("timeout")) || TIMEOUT_MS;
  SUBNET_SAMPLE_SIZE = parseInt(params.get("sn_sample_size")) || SUBNET_SAMPLE_SIZE;
  SUBNET_ALIVE_MIN = parseInt(params.get("sn_alive_min")) || SUBNET_ALIVE_MIN;

  let sn_only_24_prefix = params.get("sn_only_24_prefix");
  if (sn_only_24_prefix !== null) {
    SUBNET_ONLY_24_PREFIX = sn_only_24_prefix === "true";
  }
})();

const fetchOpt = s => ({
  method: "HEAD",
  credentials: "omit",
  cache: "no-store",
  signal: s,
  redirect: "manual",
  keepalive: true
});

const cacheSubnetsButton = document.getElementById("cache-subnets-btn");
const checkSubnetsButton = document.getElementById("check-subnets-btn");
const saveButton = document.getElementById("save-btn");

const status = document.getElementById("status");
const log = document.getElementById("log");
const resultsTable = document.getElementById("results");
let resultsData = [];
let resultsCount = 0;
let cachedSubnets = {};

const logPush = (level, prefix, msg) => {
  const now = new Date();
  const ts = now.toLocaleTimeString([], { hour12: false }) + "." + now.getMilliseconds().toString().padStart(3, "0");
  log.textContent += `[${ts}] ${prefix ? prefix + "/" : ""}${level}: ${msg}\n`;
  log.scrollTop = log.scrollHeight;
};

const timeElapsed = t0 => `${(performance.now() - t0).toFixed(1)} ms`;

const getUniqueUrl = url => {
  return url.includes('?') ? `${url}&t=${Math.random()}` : `${url}?t=${Math.random()}`;
};

const checkSubnet = async (provider, cidr) => {
  const prefix = `Subnet checker[${provider} => ${cidr}]`;
  logPush("INFO", prefix, `Started`);

  const ips = getSubnetSample(cidr, SUBNET_SAMPLE_SIZE);
  const earlyAbortCtrl = new AbortController();
  const tasks = []

  const ref = { aliveCount: 0 }; // Shares between tasks.

  for (const ip of ips) {
    tasks.push(checkIpv4Host(ip, earlyAbortCtrl, ref));
  }
  const aliveCount = (await Promise.all(tasks)).filter(x => x).length;

  if (aliveCount > 0) {
    const row = resultsTable.insertRow();
    const numCell = row.insertCell();
    const providerCell = row.insertCell();
    const subnetCell = row.insertCell();

    numCell.textContent = ++resultsCount;
    providerCell.innerHTML = `<b>${provider}</b>`;

    subnetCell.textContent = aliveCount >= SUBNET_ALIVE_MIN ? `${cidr} ✅` : `${cidr} ⚠️`;
    resultsData.push({ provider, cidr, aliveCount });
  }

  logPush("INFO", prefix, `Done (alive: ${aliveCount}).`);
}

const checkSubnets = async () => {
  const t0 = performance.now();
  const prefix = `Subnets checker`;
  logPush("INFO", prefix, `Started`);

  for (let i = resultsTable.rows.length - 1; i > 0; i--) {
    resultsTable.deleteRow(i);
  }

  resultsCount = 0;
  resultsData = [];
  const subnetsTotal = Object.values(cachedSubnets).flat().length;
  let subnetsChecked = 0;

  checkSubnetsButton.disabled = true;
  cacheSubnetsButton.disabled = true;
  saveButton.disabled = true;
  checkSubnetsButton.textContent = "...";
  status.className = "status-working";

  for (const [provider, subnets] of Object.entries(cachedSubnets)) {
    for (const s of subnets) {
      status.textContent = `Subnets checking (${subnetsChecked++}/${subnetsTotal}) ⏰`;
      await checkSubnet(provider, s);
    }
  }

  status.textContent = "Ready (cached ⚡)";
  status.className = "status-ready";
  checkSubnetsButton.disabled = false;
  cacheSubnetsButton.disabled = false;
  checkSubnetsButton.textContent = "Check 🔥";

  if (resultsCount > 0) {
    saveButton.disabled = false;
  }

  console.log("result data", resultsData);
  logPush("INFO", prefix, `Done (found: ${resultsCount}, elapsed: ${timeElapsed(t0)}).`);
}

const cacheSubnets = async () => {
  const t0 = performance.now();
  const prefix = `Subnets cacher`;
  log.textContent = "";
  sessionStorage.clear();

  logPush("INFO", prefix, `Started`);

  status.textContent = "Subnets caching ⏰";
  status.className = "status-working";
  cacheSubnetsButton.disabled = true;
  cacheSubnetsButton.textContent = "...";

  for (let i = results.rows.length - 1; i > 0; i--) {
    results.deleteRow(i);
  }

  try {
    for (let t of TEST_SUITE) {
      const r = await fetchProviderIpv4Subnets(t);
      cachedSubnets[t.name] = r;
    }

    console.log("cached subnets", cachedSubnets);
    localStorage.setItem("ipv4-whitelisted-subnets_cachedSubnets", JSON.stringify(cachedSubnets));

    checkSubnetsButton.disabled = false;
    cacheSubnetsButton.disabled = false;
    cacheSubnetsButton.textContent = "Cache";
    status.className = "status-ready";
    status.textContent = "Ready (cached ⚡)";

    logPush("INFO", prefix, `Cached in ${timeElapsed(t0)}.`);
  } catch (e) {
    checkSubnetsButton.disabled = true;
    cacheSubnetsButton.disabled = false;
    cacheSubnetsButton.textContent = "Cache";
    status.textContent = "Unexpected caching error ⚠️";
    status.className = "status-error";
    logPush("ERR", prefix, `Unexpected caching error => ${e}`);
  }
};

// Returns N random unique hosts from a subnet based on CIDR.
const getSubnetSample = (cidr, n) => {
  const [ip, maskStr] = cidr.split('/');

  const ipToUint32 = s => {
    const [a, b, c, d] = s.split('.').map(Number);
    return a * 2 ** 24 + b * 2 ** 16 + c * 2 ** 8 + d;
  };

  const uint32ToIp = x => {
    const a = Math.floor(x / 2 ** 24) & 255;
    const b = Math.floor(x / 2 ** 16) & 255;
    const c = Math.floor(x / 2 ** 8) & 255;
    const d = x & 255;
    return `${a}.${b}.${c}.${d}`;
  };

  const blockSize = 2 ** (32 - Number(maskStr));
  const swap = new Map();
  const result = new Array(n);

  for (let i = 0; i < n; i++) {
    const r = i + Math.floor(Math.random() * (blockSize - i - 2));

    const pick = swap.has(r) ? swap.get(r) : r;
    swap.set(r, swap.has(i) ? swap.get(i) : i);

    result[i] = uint32ToIp(Math.floor(ipToUint32(ip) / blockSize) * blockSize + pick + 1);
  }

  return result;
};

// Any response from the server (including HTTP or CORS errors) is considered correct. Only a timeout is a signal of restrictions.
const checkIpv4Host = async (ip, earlyAbortCtrl, ref) => {
  if (ref.aliveCount >= SUBNET_ALIVE_MIN) {
    earlyAbortCtrl.abort();
    logPush("INFO", prefix, `Early abort ⏭️`);
    return false;
  }

  const timeoutCtrl = new AbortController();
  const t = setTimeout(() => timeoutCtrl.abort(), TIMEOUT_MS);
  const prefix = `Host checker[${ip}]`;

  const abortSignals = AbortSignal.any([earlyAbortCtrl.signal, timeoutCtrl.signal]);

  let result = true;
  try {
    logPush("INFO", prefix, `Started`);

    await fetch(getUniqueUrl(`https://${ip}/`), fetchOpt(abortSignals));
  } catch (e) {
    if (e.name === "AbortError") result = false;
  } finally {
    clearTimeout(t);
  }

  if (result) {
    ref.aliveCount++;
  }

  if (ref.aliveCount >= SUBNET_ALIVE_MIN) {
    earlyAbortCtrl.abort();
  }

  logPush("INFO", prefix, `${result ? "Alive ✅" : (earlyAbortCtrl.signal.aborted ? "Early abort ⏭️" : "Dead 💀")}.`);
  return result;
}

const isIpv4Cidr = s => s.includes('.') && s.includes('/');

const fetchAsIpv4Subnets = async (asn) => {
  const prefix = `AS IPv4 subnets fetcher[AS${asn}]`;
  const RIPE_API_URL = "https://stat.ripe.net/data/";

  try {
    logPush("INFO", prefix, `Started`);
    const prefixes = (await (await fetch(RIPE_API_URL + "announced-prefixes/data.json?resource=" + asn)).json()).data.prefixes
      .map(x => x.prefix)
      .filter(x => isIpv4Cidr(x));

    logPush("INFO", prefix, `Done (total: ${prefixes.length}).`);
    return prefixes;
  } catch (err) {
    throw prefix + err;
  }
}

const fetchProviderIpv4Subnets = async (provider) => {
  const prefix = `Provider IPv4 subnets fetcher[${provider.name}]`;

  logPush("INFO", prefix, `Started`);
  const tasks = [];
  for (let i = 0; i < provider.asns.length; i++) {
    tasks.push(fetchAsIpv4Subnets(provider.asns[i]));
  }

  const all = (await Promise.all(tasks)).flat();
  const merged = [...new Set(all)];
  let suitable = merged;

  if (SUBNET_ONLY_24_PREFIX) {
    suitable = merged.filter(x => x.split('/')[1] == "24");
  }

  logPush("INFO", prefix, `Done (all: ${all.length}, merged: ${merged.length}, suitable: ${suitable.length}).`);
  return suitable;
}

const saveResults = () => {
  const content = "provider;cidr;aliveCount\n" + resultsData.map(x => `${x.provider};${x.cidr};${x.aliveCount}`).join("\n");
  const blob = new Blob([content], {
    type: "text/csv;charset=utf-8"
  });

  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = `ipv4-whitelisted-subnets-${new Date().toISOString()}.csv`;
  a.click();
  URL.revokeObjectURL(url);
};

cacheSubnetsButton.onclick = () => {
  cacheSubnets();
};

checkSubnetsButton.onclick = () => {
  checkSubnets();
};

saveButton.onclick = () => {
  saveResults();
};

document.addEventListener("DOMContentLoaded", async () => {
  const v = localStorage.getItem("ipv4-whitelisted-subnets_cachedSubnets");
  if (v) {
    cachedSubnets = JSON.parse(v);
    const total = Object.values(cachedSubnets).flat().length;
    console.log("cached subnets", cachedSubnets);
    checkSubnetsButton.disabled = false;
    status.textContent = "Ready (cached ⚡)";
    status.className = "status-ready";
    logPush("INFO", null, `Cached subnets loaded (providers: ${Object.keys(cachedSubnets).length}, total subnets: ${total}).`);
    return;
  }

  logPush("INFO", null, `Cached subnets not found.`);
});