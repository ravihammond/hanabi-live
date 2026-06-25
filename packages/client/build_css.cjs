const fs = require("node:fs");
const path = require("node:path");
const CleanCSS = require("clean-css");
const { VERSION } = require("../data/src/version");

const repoRoot = path.resolve(__dirname, "..", "..");
const cssDir = path.join(repoRoot, "public", "css");
const cssLibDir = path.join(cssDir, "lib");
const outputDir = path.join(__dirname, "grunt_output");
const bundleFilename = `main.${VERSION}.min.css`;

const sourcePaths = [
  path.join(cssLibDir, "fontawesome.min.css"),
  path.join(cssLibDir, "solid.min.css"),
  path.join(cssLibDir, "tooltipster.bundle.min.css"),
  path.join(cssLibDir, "tooltipster-sideTip-shadow.min.css"),
  path.join(cssLibDir, "alpha.css"),
  path.join(cssDir, "hanabi.css"),
];

fs.mkdirSync(outputDir, { recursive: true });

const concatenatedCSS = sourcePaths
  .map((sourcePath) => fs.readFileSync(sourcePath, "utf8"))
  .join("\n");
const mainCSSPath = path.join(outputDir, "main.css");
fs.writeFileSync(mainCSSPath, concatenatedCSS, "utf8");

const minified = new CleanCSS({ level: 2 }).minify(concatenatedCSS);
if (minified.errors.length > 0) {
  throw new Error(minified.errors.join("\n"));
}
for (const warning of minified.warnings) {
  console.warn(warning);
}

fs.writeFileSync(path.join(outputDir, bundleFilename), minified.styles, "utf8");
console.log(`>> 1 file created. ${concatenatedCSS.length} B -> ${minified.styles.length} B`);
