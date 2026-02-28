export const COMMIT_BYTES = 16;
export const COMMIT_BITS = COMMIT_BYTES * 8;

export const TIMESTAMP_BITS = 23;
export const ASN_BITS = 32;
export const EPOCH_MS = Date.UTC(2026, 0, 1);

export const ENDPOINT_STATE_BITS = 4;
export const ALIVE_CARDINALITY = 3;
export const DPI_CARDINALITY = 5;

export const REPO = "hyperion-cs/dpi-checkers";
export const SELFCHECK_ID = "US.GH-HPRN";

// Just for the aesthetics of share links; not cryptography.
export const XOR_KEY = "564qHVfzK4NMajBz2NVtaIWwyQgXrRd5"
  + "ygpHOo5AeSy5iqFyV4f9Ekd0o26YG7k1"
  + "z18r8jKhu8AaKvBcOkZ2e0WYc7H8UGyp"
  + "lUgd4WbvguXUnrjMmXEZXCPndiIIUabj";

export const getCommitHex = (buf) => {
  const commit = readBits(buf, 0, COMMIT_BITS);
  return commit.toString(16);
}

export const getLastCommitBigint = async () => {
  const truncSize = 32;
  const url = `https://api.github.com/repos/${REPO}/commits?per_page=1`;
  const sha = (await (await fetch(url)).json())[0].sha;
  console.log("last commit sha", sha);
  return BigInt("0x" + sha.slice(0, truncSize));
};

export const setXor = (data, key, skip) => {
  for (let i = skip; i < data.length; i++) {
    data[i] ^= key.charCodeAt(i % key.length);
  }
};

// Write BigInt to Uint8Array
export const writeBits = (buf, bitOffset, bitLength, value) => {
  for (let i = 0; i < bitLength; i++) {
    const bit = Number((value >> BigInt(i)) & 1n);
    const idx = bitOffset + bitLength - 1 - i;
    const byteIndex = idx >> 3;
    const bitIndex = 7 - (idx & 7);
    if (bit) {
      buf[byteIndex] |= 1 << bitIndex;
    } else {
      buf[byteIndex] &= ~(1 << bitIndex);
    }
  }
};

// Read Uint8Array to BigInt
export const readBits = (buf, bitOffset, bitLength) => {
  let result = 0n;
  for (let i = 0; i < bitLength; i++) {
    const idx = bitOffset + i;
    const byteIndex = idx >> 3;
    const bitIndex = 7 - (idx & 7);
    const bit = (buf[byteIndex] >> bitIndex) & 1;
    result = (result << 1n) | BigInt(bit);
  }
  return result;
};
