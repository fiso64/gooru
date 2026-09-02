const hexColorPattern = /^#[0-9a-f]{6}$/i;
const darkAccentInk = '#241f12';
const lightAccentInk = '#fffdf6';

export type AccentTheme = {
  accent: string;
  accentInk: string;
};

export function accentTheme(accentColor: string): AccentTheme | null {
  const color = accentColor.trim();
  if (!hexColorPattern.test(color)) return null;

  const accentLuminance = hexLuminance(color);
  const darkContrast = contrastRatio(accentLuminance, hexLuminance(darkAccentInk));
  const lightContrast = contrastRatio(accentLuminance, hexLuminance(lightAccentInk));

  return {
    accent: color.toLowerCase(),
    accentInk: darkContrast >= lightContrast ? darkAccentInk : lightAccentInk
  };
}

function hexLuminance(color: string): number {
  return relativeLuminance(
    Number.parseInt(color.slice(1, 3), 16),
    Number.parseInt(color.slice(3, 5), 16),
    Number.parseInt(color.slice(5, 7), 16)
  );
}

function contrastRatio(first: number, second: number): number {
  const lighter = Math.max(first, second);
  const darker = Math.min(first, second);
  return (lighter + 0.05) / (darker + 0.05);
}

function relativeLuminance(red: number, green: number, blue: number): number {
  const channels = [red, green, blue].map((value) => {
    const channel = value / 255;
    return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
}
