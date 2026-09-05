let protectedReadTransport = false;

export function setProtectedReadTransport(enabled: boolean) {
  protectedReadTransport = enabled;
}

export function useProtectedReadTransport() {
  return protectedReadTransport;
}
