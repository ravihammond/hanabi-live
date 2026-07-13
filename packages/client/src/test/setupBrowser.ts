/// <reference types="node" />

import { readFileSync } from "node:fs";
import path from "node:path";

const mainTemplatePath = path.resolve(
  __dirname,
  "../../../..",
  "server/src/views/main.tmpl",
);

document.body.innerHTML = readFileSync(mainTemplatePath, "utf8");

// JSDOM does not implement canvas rendering. Konva only needs these two methods while constructing
// the UI controls exercised by browser interaction tests.
Object.defineProperty(HTMLCanvasElement.prototype, "getContext", {
  configurable: true,
  value: () => ({
    measureText: (text: string) => ({ width: text.length * 10 }),
    scale: () => undefined,
  }),
});
