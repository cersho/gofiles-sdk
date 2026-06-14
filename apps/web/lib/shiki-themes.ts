import type { ThemeRegistration } from "shiki";

export const goBlueDarkTheme: ThemeRegistration = {
  name: "go-blue-dark",
  type: "dark",
  bg: "#0B0F14",
  fg: "#E5EDF5",
  colors: {
    "editor.background": "#0B0F14",
    "editor.foreground": "#E5EDF5",
  },
  settings: [
    {
      settings: {
        background: "#0B0F14",
        foreground: "#E5EDF5",
      },
    },
    {
      scope: ["keyword", "storage", "constant.language"],
      settings: {
        foreground: "#38BDF8",
      },
    },
    {
      scope: ["entity.name.function", "support.function"],
      settings: {
        foreground: "#67E8F9",
      },
    },
    {
      scope: ["entity.name.type", "support.type", "storage.type"],
      settings: {
        foreground: "#BAE6FD",
      },
    },
    {
      scope: ["variable", "variable.other", "variable.parameter"],
      settings: {
        foreground: "#DCEBFA",
      },
    },
    {
      scope: ["string", "constant.other.symbol"],
      settings: {
        foreground: "#8BE9C1",
      },
    },
    {
      scope: ["constant.numeric", "constant"],
      settings: {
        foreground: "#60A5FA",
      },
    },
    {
      scope: ["comment", "punctuation.definition.comment"],
      settings: {
        foreground: "#64748B",
        fontStyle: "italic",
      },
    },
    {
      scope: ["punctuation", "operator"],
      settings: {
        foreground: "#8AA2B8",
      },
    },
  ],
};

export const goBlueLightTheme: ThemeRegistration = {
  name: "go-blue-light",
  type: "light",
  bg: "#F8FAFC",
  fg: "#172033",
  colors: {
    "editor.background": "#F8FAFC",
    "editor.foreground": "#172033",
  },
  settings: [
    {
      settings: {
        background: "#F8FAFC",
        foreground: "#172033",
      },
    },
    {
      scope: ["keyword", "storage", "constant.language"],
      settings: {
        foreground: "#007A99",
      },
    },
    {
      scope: ["entity.name.function", "support.function"],
      settings: {
        foreground: "#0087A8",
      },
    },
    {
      scope: ["entity.name.type", "support.type", "storage.type"],
      settings: {
        foreground: "#075985",
      },
    },
    {
      scope: ["variable", "variable.other", "variable.parameter"],
      settings: {
        foreground: "#26384F",
      },
    },
    {
      scope: ["string", "constant.other.symbol"],
      settings: {
        foreground: "#087F5B",
      },
    },
    {
      scope: ["constant.numeric", "constant"],
      settings: {
        foreground: "#2563EB",
      },
    },
    {
      scope: ["comment", "punctuation.definition.comment"],
      settings: {
        foreground: "#64748B",
        fontStyle: "italic",
      },
    },
    {
      scope: ["punctuation", "operator"],
      settings: {
        foreground: "#60758C",
      },
    },
  ],
};
