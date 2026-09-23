import type { Config } from "tailwindcss";

export default {
  content: ["./index.html", "./src/**/*.{ts,tsx}"],
  theme: {
    extend: {
      colors: {
        ink: "#162433",
        muted: "#607080",
        line: "#d7e0e8",
        canvas: "#f4f7f9",
        navy: "#102a43",
        teal: "#087f8c",
        amber: "#9a6700",
        danger: "#b42318",
      },
      fontFamily: {
        sans: ["Inter", "ui-sans-serif", "system-ui", "sans-serif"],
        mono: ["ui-monospace", "SFMono-Regular", "Menlo", "monospace"],
      },
    },
  },
  plugins: [],
} satisfies Config;
