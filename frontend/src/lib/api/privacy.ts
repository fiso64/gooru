let protectedReadTransport = false;
let opaqueURLState = false;

export type ProtectedReadTransport<T> = {
  clearQuery: () => T;
  protectedBody: () => T;
};

export function setProtectedReadTransport(enabled: boolean) {
  protectedReadTransport = enabled;
}

// Privacy-sensitive reads declare both transports and let this boundary choose
// between them. Feature/API methods should not inspect protected mode directly:
// ordinary mode keeps its query-string fast path, while protected mode can use
// a request body without exposing private values in the URL.
export function selectProtectedReadTransport<T>(transport: ProtectedReadTransport<T>): T {
  return protectedReadTransport ? transport.protectedBody() : transport.clearQuery();
}

export function setOpaqueURLState(enabled: boolean) {
  opaqueURLState = enabled;
}

export function useOpaqueURLState() {
  return opaqueURLState;
}
