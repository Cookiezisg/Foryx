#!/usr/bin/env node
/* Foryx demo — Level-1 机械门禁（仿后端 standard_test.go 的“机械守卫防回退”）。
   无依赖、纯 Node。扫描 demo/core 与 demo/features 下的 *.js / *.css：
     1) 裸 hex 颜色          → 必须走 tokens.css 的语义色 token
     2) 裸 px / ms（非 0）   → 必须走密度/动效 token
     3) 在别处定义 --自定义属性 → 只能在 tokens.css 定义
     4) 注册非 an- 的 custom element 标签
   值类规则（hex/px/ms）只查【CSS 上下文】：跳过单/双引号字符串字面量内容（那是数据，如执行耗时 "842ms"），
   仍命中反引号 static css 模板与 .css 文件里的裸值——杜绝对数据的伪报（反校验剧场）。
   豁免：core/tokens.css（值源）+ core/reset.css（light-dom 基座）。
   用法：node demo/tools/lint.mjs   （CI/pre-commit：非零退出 = 失败） */
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative, extname } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = join(fileURLToPath(import.meta.url), "..", "..");        // demo/
const SCAN_DIRS = ["core", "features"].map((d) => join(ROOT, d));
const EXEMPT = new Set(["core/tokens.css", "core/reset.css"]);       // 值源 / 基座层
const violations = [];

function walk(dir) {
  let out = [];
  let entries;
  try { entries = readdirSync(dir); } catch { return out; }
  for (const name of entries) {
    const p = join(dir, name);
    if (statSync(p).isDirectory()) out = out.concat(walk(p));
    else if ([".js", ".css", ".mjs"].includes(extname(p))) out.push(p);
  }
  return out;
}

// 去注释（/* */ 跨行 + // 行尾），保留行号；返回逐行的“代码部分”。
function stripComments(text) {
  const lines = text.split("\n");
  let inBlock = false;
  return lines.map((line) => {
    let out = "";
    for (let i = 0; i < line.length; i++) {
      if (inBlock) {
        if (line[i] === "*" && line[i + 1] === "/") { inBlock = false; i++; }
        continue;
      }
      if (line[i] === "/" && line[i + 1] === "*") { inBlock = true; i++; continue; }
      if (line[i] === "/" && line[i + 1] === "/") break;            // 行尾注释
      out += line[i];
    }
    return out;
  });
}

// 把单/双引号字符串字面量内容抹成空格（保留长度与行号、保留反引号模板）——值类规则只查 CSS 上下文，不查数据串。
function stripQuoted(line) {
  let out = "", q = null;
  for (let i = 0; i < line.length; i++) {
    const c = line[i];
    if (q) {
      if (c === "\\") { out += "  "; i++; continue; }   // 跳过转义字符
      if (c === q) { q = null; out += c; continue; }
      out += " ";
    } else if (c === '"' || c === "'") { q = c; out += c; }
    else out += c;
  }
  return out;
}

const RULES = [
  { id: "bare-hex", cssValue: true, re: /#[0-9a-fA-F]{3,8}\b/g, msg: "裸 hex 颜色 → 改用 tokens.css 语义色" },
  {
    id: "bare-px", cssValue: true, re: /(?<![\w.#])\d*\.?\d+px\b/g, msg: "裸 px → 改用密度 token（--row/--gap/--sp-* …）",
    keep: (m) => parseFloat(m) !== 0,
  },
  {
    id: "bare-ms", cssValue: true, re: /(?<![\w.#])\d*\.?\d+ms\b/g, msg: "裸 ms → 改用动效 token（--d-fast/--d-mid …）",
    keep: (m) => parseFloat(m) !== 0,
  },
  { id: "var-def", re: /(?<![\w-])--[a-z][\w-]*\s*:/g, msg: "在 tokens.css 之外定义自定义属性 → 值只能在 tokens.css 登记" },
  {
    id: "non-an-tag",
    re: /(?:static\s+tag\s*=|customElements\.define\s*\(\s*)["'`]([a-z][\w-]*)["'`]/g,
    msg: "注册了非 an- 前缀的 custom element",
    keep: (_m, g1) => g1 && !g1.startsWith("an-"),
  },
];

for (const dir of SCAN_DIRS) {
  for (const file of walk(dir)) {
    const rel = relative(ROOT, file).split("\\").join("/");
    if (EXEMPT.has(rel)) continue;
    if (rel.includes("/vendor/") || rel.startsWith("vendor/")) continue;  // 第三方 vendored 资产豁免
    const codeLines = stripComments(readFileSync(file, "utf8"));
    codeLines.forEach((code, idx) => {
      const cssCtx = stripQuoted(code);   // 值类规则的扫描面（已抹掉引号串内容）
      for (const rule of RULES) {
        const target = rule.cssValue ? cssCtx : code;
        rule.re.lastIndex = 0;
        let m;
        while ((m = rule.re.exec(target)) !== null) {
          if (rule.keep && !rule.keep(m[0], m[1])) continue;
          violations.push({ file: rel, line: idx + 1, rule: rule.id, msg: rule.msg, hit: m[0].trim() });
        }
      }
    });
  }
}

if (violations.length === 0) {
  console.log("✓ demo lint 通过：无裸值 / 无越界 token 定义 / 标签前缀合规。");
  process.exit(0);
}
console.error(`✗ demo lint 失败：${violations.length} 处违规\n`);
for (const v of violations) {
  console.error(`  ${v.file}:${v.line}  [${v.rule}]  «${v.hit}»  — ${v.msg}`);
}
process.exit(1);
