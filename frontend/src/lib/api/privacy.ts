let protectedReadTransport = false;
let opaqueURLState = false;

export function setProtectedReadTransport(enabled: boolean) { protectedReadTransport = enabled; }
export function useProtectedReadTransport() { return protectedReadTransport; }
export function setOpaqueURLState(enabled: boolean) { opaqueURLState = enabled; }
export function useOpaqueURLState() { return opaqueURLState; }
