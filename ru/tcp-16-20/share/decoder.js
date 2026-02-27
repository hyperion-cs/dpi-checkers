
const _numToUtcNow = (v, epoch) => new Date(epoch + Number(v) * 60000);


const decodeItem = (aliveCardinality, state) => {
  const alive = state % aliveCardinality;
  const dpi = Math.floor(state / aliveCardinality);
  return { alive, dpi };
}

export const decodeShare = async (buf, count) => {
  // короче, эта функция у каждого коммита своя...
  // тут подключаем свою версию хелперов (глядя в хедер), т.е. пусть надо будет заменить на иной
  const h = await import('./helpers.js');
  const commit = h.readBits(buf, 0, h.COMMIT_BITS)
  const commitHex = commit.toString(16);
  const tsRaw = h.readBits(buf, h.COMMIT_BITS, h.TIMESTAMP_BITS);
  const ts = _numToUtcNow(tsRaw, h.EPOCH_MS);
  const asn = Number(h.readBits(buf, h.COMMIT_BITS + h.TIMESTAMP_BITS, h.ASN_BITS));

  // тут мы читаем архивный файл сьюитов, на основе коммита. но пока и так сойдет.
  const endpoints = await (await fetch("./suite.v2.json")).json();
  // его надо обязательно сортировать по id
  endpoints.sort((a, b) => a.id < b.id ? -1 : (a.id > b.id ? 1 : 0));

  const items = []
  let itemOffset = h.COMMIT_BITS + h.TIMESTAMP_BITS + h.ASN_BITS;
  for (let i = 0; i < endpoints.length; i++) {
    const itemRaw = h.readBits(buf, itemOffset, h.ENDPOINT_STATE_BITS);
    const decodedItem = decodeItem(h.ALIVE_CARDINALITY, Number(itemRaw));
    items.push({ ...decodedItem, ...endpoints[i] });
    itemOffset += h.ENDPOINT_STATE_BITS;
  }

  items.sort((a, b) => (a.id == h.SELFCHECK_ID || a.provider < b.provider) ? -1 : (a.provider > b.provider ? 1 : 0));
  return { commitHex, ts, asn, items };
};
