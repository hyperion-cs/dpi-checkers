
const _numToUtcNow = (v, epoch) => new Date(epoch + Number(v) * 60000);

export const decodeShare = async (buf) => {
  // короче, эта функция у каждого коммита своя...
  // тут подключаем свою версию хелперов (глядя в хедер), т.е. пусть надо будет заменить на иной
  const h = await import('./helpers.js');
  const commit = h.readBits(buf, 0, h.COMMIT_BITS)
  const commitHex = commit.toString(16);
  const tsRaw = h.readBits(buf, h.COMMIT_BITS, h.TIMESTAMP_BITS);
  const ts = _numToUtcNow(tsRaw, h.EPOCH_MS);
  const asn = Number(h.readBits(buf, h.COMMIT_BITS+h.TIMESTAMP_BITS, h.ASN_BITS));

  const data = { commitHex, ts, asn};
  console.log(data);
  return data;
};
