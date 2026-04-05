import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./app/**/*.{ts,tsx}",
    "./components/**/*.{ts,tsx}",
    "./lib/**/*.{ts,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        surface: {
          DEFAULT: "#0f1117",
          card: "#161b27",
          border: "#1e2535",
        },
        brand: {
          DEFAULT: "#4f8ef7",
          dim: "#2d5cbf",
        },
      },
    },
  },
  plugins: [],
};

export default config;
