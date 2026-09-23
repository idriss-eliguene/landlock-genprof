export function canonicalImageIdentity(value?: string): string | undefined {
  if (!value) return undefined;
  const match = value.match(/(?:^|@)(sha256:[0-9a-fA-F]{64})$/);
  return match?.[1];
}
