const hexColorPattern = /^#[0-9a-f]{6}$/i;

export type AccentTheme = {
  accent: string;
  accentInk: string;
};

export function accentTheme(accentColor: string): AccentTheme | null {
  const color = accentColor.trim();
  if (!hexColorPattern.test(color)) return null;

  const red = Number.parseInt(color.slice(1, 3), 16);
  const green = Number.parseInt(color.slice(3, 5), 16);
  const blue = Number.parseInt(color.slice(5, 7), 16);
  const luminance = relativeLuminance(red, green, blue);

  return {
    accent: color.toLowerCase(),
    accentInk: luminance > 0.38 ? '#241f12' : '#fffdf6'
  };
}

function relativeLuminance(red: number, green: number, blue: number): number {
  const channels = [red, green, blue].map((value) => {
    const channel = value / 255;
    return channel <= 0.04045 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * channels[0] + 0.7152 * channels[1] + 0.0722 * channels[2];
}
