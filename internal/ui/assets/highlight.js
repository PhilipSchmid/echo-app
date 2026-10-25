// Small, offline lexers for the UI's JSON output and generated shell commands.
export function codeTokens(text, language = "auto") {
  const plain = [{ text }];
  // shortcut: large or token-dense previews stay plain; use a worker if richer highlighting is needed.
  if (text.length > 65536) return plain;
  if (language === "auto" || language === "json") {
    try { JSON.parse(text); language = "json"; } catch { return plain; }
  }
  let pattern;
  if (language === "json") {
    pattern = /(?<key>"(?:\\.|[^"\\])*"(?=\s*:))|(?<string>"(?:\\.|[^"\\])*")|(?<number>-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)|(?<literal>\b(?:true|false|null)\b)|(?<punctuation>[{}\[\],:])/g;
  } else if (["bash", "shell", "sh"].includes(language)) {
    pattern = /(?<comment>#[^\n]*)|(?<string>'[^']*'|"(?:\\[\s\S]|[^"\\])*")|(?<variable>\$(?:\{[^}]*\}|[a-zA-Z_]\w*|[0-9?@#*!$-]))|(?<option>--?[a-zA-Z][\w-]*)|(?<operator>\\\r?\n|&&|\|\||[|;&<>])|(?<escape>\\[\s\S])|(?<word>[^\s'"$\\|;&<>]+)/g;
  } else return plain;
  const tokens = [];
  let offset = 0;
  let command = true;
  for (const match of text.matchAll(pattern)) {
    const gap = text.slice(offset, match.index);
    if (gap) tokens.push({ text: gap });
    if (gap.includes("\n")) command = true;
    let kind = Object.keys(match.groups).find((key) => match.groups[key] !== undefined);
    if (kind === "word") {
      kind = command ? "command" : /^\d+(?:\.\d+)?$/.test(match[0]) ? "number" : undefined;
      command = false;
    } else if (kind === "operator" && /^[|;&]/.test(match[0])) command = true;
    else if (kind === "string" || kind === "variable") command = false;
    tokens.push({ text: match[0], kind });
    if (tokens.length > 4096) return plain;
    offset = match.index + match[0].length;
  }
  if (offset < text.length) tokens.push({ text: text.slice(offset) });
  return tokens;
}

export function renderCode(target, text, language = "auto") {
  const fragment = document.createDocumentFragment();
  for (const token of codeTokens(text, language)) {
    if (!token.kind) { fragment.append(document.createTextNode(token.text)); continue; }
    const span = document.createElement("span");
    span.className = "syntax-" + token.kind;
    span.textContent = token.text;
    fragment.append(span);
  }
  target.replaceChildren(fragment);
}
