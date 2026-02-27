
const _numToUtcNow = (v, epoch) => new Date(epoch + Number(v) * 60000);


const decodeItem = (aliveCardinality, state) => {
  const alive = state % aliveCardinality;
  const dpi = Math.floor(state / aliveCardinality);
  return { alive, dpi };
}

export const decodeShare = async (repo, commitHex, buf) => {
  let helpersUrl = `https://raw.githubusercontent.com/${repo}/${commitHex}/ru/tcp-16-20/share/helpers.js`;
  let endpointsUrl = `https://raw.githubusercontent.com/${repo}/${commitHex}/ru/tcp-16-20/suite.v2.json`;

  if (DEBUG) {
    helpersUrl = "./helpers.js";
    endpointsUrl = "./suite.v2.json"
  }
  const h = await import(helpersUrl);

  const tsRaw = h.readBits(buf, h.COMMIT_BITS, h.TIMESTAMP_BITS);
  const ts = _numToUtcNow(tsRaw, h.EPOCH_MS);
  const asn = Number(h.readBits(buf, h.COMMIT_BITS + h.TIMESTAMP_BITS, h.ASN_BITS));

  const endpoints = await (await fetch(endpointsUrl)).json();
  endpoints.sort((a, b) => a.id < b.id ? -1 : (a.id > b.id ? 1 : 0)); // guaranteed order of sequence

  const items = []
  let itemOffset = h.COMMIT_BITS + h.TIMESTAMP_BITS + h.ASN_BITS;
  for (let i = 0; i < endpoints.length; i++) {
    const itemRaw = h.readBits(buf, itemOffset, h.ENDPOINT_STATE_BITS);
    const decodedItem = decodeItem(h.ALIVE_CARDINALITY, Number(itemRaw));
    items.push({ ...decodedItem, ...endpoints[i] });
    itemOffset += h.ENDPOINT_STATE_BITS;
  }

  // does not affect decoding
  // just so it's roughly the same as the original
  const sortFunc = (a, b) => {
    if (a.id == h.SELFCHECK_ID) return -1;
    const aprovider = a.provider.toLowerCase();
    const bprovider = b.provider.toLowerCase();
    if (aprovider < bprovider) return -1;
    if (aprovider > bprovider) return 1;
    return 0;
  };

  items.sort(sortFunc);
  return { commitHex, ts, asn, items };
};
