import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const css = await readFile(new URL("../internal/ui/assets/app.css", import.meta.url), "utf8");
function palette(selector) {
  const body = css.match(selector)[1];
  return Object.fromEntries([...body.matchAll(/--([\w-]+)\s*:\s*([^;]+);/g)].map((match) => [match[1], match[2].trim()]));
}
const light = palette(/:root\s*\{([^}]+)\}/);
const dark = { ...light, ...palette(/html\[data-theme="dark"\]\s*\{([^}]+)\}/) };
const systemDark = { ...light, ...palette(/html\[data-theme="system"\]\s*\{([^}]+)\}/) };
function luminance(hex) {
  assert.match(hex, /^#(?:[\da-f]{3}|[\da-f]{6})$/i);
  if (hex.length === 4) hex = "#" + [...hex.slice(1)].map((character) => character.repeat(2)).join("");
  const rgb = hex.slice(1).match(/../g).map((value) => parseInt(value, 16) / 255).map((value) => value <= .04045 ? value / 12.92 : ((value + .055) / 1.055) ** 2.4);
  return rgb[0] * .2126 + rgb[1] * .7152 + rgb[2] * .0722;
}
function contrast(first, second) {
  const values = [luminance(first), luminance(second)].sort((a, b) => b - a);
  return (values[0] + .05) / (values[1] + .05);
}
function mix(first, second, weight) {
  const channels = (hex) => hex.slice(1).match(/../g).map((value) => parseInt(value, 16));
  const a = channels(first), b = channels(second);
  return "#" + a.map((value, index) => Math.round(value * weight + b[index] * (1 - weight)).toString(16).padStart(2, "0")).join("");
}

test("code blocks use a light palette in light mode and the same dark palette for explicit and system dark", () => {
  assert.ok(luminance(light["code-bg"]) > .8, "Light code blocks must have a light background");
  assert.ok(luminance(dark["code-bg"]) < .1, "Dark code blocks must have a dark background");
  for (const key of Object.keys(light).filter((key) => /^(code-|syntax-)/.test(key))) {
    assert.notEqual(light[key], dark[key], key);
    assert.equal(systemDark[key], dark[key], key);
  }
});

test("code text, syntax, feedback, selection, and focus remain readable in both themes", () => {
  for (const colors of [light, dark]) {
    for (const background of ["code-bg", "code-toolbar"]) {
      for (const foreground of ["code-text", "code-muted", "code-success", "code-error", "syntax-key", "syntax-string", "syntax-number", "syntax-literal", "syntax-muted"]) {
        assert.ok(contrast(colors[foreground], colors[background]) >= 4.5, `${foreground} on ${background}`);
      }
      assert.ok(contrast(colors.focus, colors[background]) >= 3, `Focus on ${background}`);
    }
    assert.ok(contrast(colors["code-selection-text"], colors["code-selection"]) >= 4.5, "Selected code text");
  }
});

test("pastel control surfaces keep readable text and icons in both themes", () => {
  for (const colors of [light, dark]) {
    for (const [foreground, background] of [
      ["nav-active-text", "nav-active"], ["muted", "nav-tray"],
      ["pastel-blue-ink", "pastel-blue"], ["pastel-lilac-ink", "pastel-lilac"], ["pastel-mint-ink", "pastel-mint"],
      ["pastel-blue-ink", "chrome"], ["muted", "chrome"],
      ["warning", "warning-bg"], ["text", "pastel-blue"], ["muted", "pastel-blue"],
      ["copper-text", "warning-bg"],
      ["text", "pastel-lilac"], ["muted", "pastel-lilac"],
      ["text", "pastel-mint"], ["muted", "pastel-mint"],
      ["text", "warning-bg"], ["muted", "warning-bg"],
      ["text", "surface-subtle"], ["muted", "surface-subtle"],
      ["text", "clay-top"], ["text", "clay-bottom"], ["muted", "clay-top"], ["muted", "clay-bottom"],
      ["action-text", "action"], ["action-text", "action-hover"]
    ]) assert.ok(contrast(colors[foreground], colors[background]) >= 4.5, `${foreground} on ${background}`);
  }
  for (const key of Object.keys(light).filter((key) => /^(clay-|nav-|pastel-)/.test(key))) assert.equal(systemDark[key], dark[key], key);
});

test("dropdown tray text and keyboard focus remain clear on pastel option surfaces", () => {
  for (const colors of [light, dark]) {
    assert.ok(contrast(colors.text, colors["nav-tray"]) >= 4.5, "Dropdown tray text");
    for (const surface of ["pastel-blue", "pastel-lilac"]) {
      assert.ok(contrast(colors.focus, colors[surface]) >= 3, `Keyboard focus on ${surface}`);
    }
  }
});

test("clay code toolbars and copy controls keep readable labels and feedback", () => {
  for (const colors of [light, dark]) {
    for (const background of ["clay-top", "clay-bottom", "pastel-blue"]) {
      for (const foreground of ["code-text", "code-muted", "code-success", "code-error"]) {
        assert.ok(contrast(colors[foreground], colors[background]) >= 4.5, `${foreground} on ${background}`);
      }
    }
  }
});

test("body text, hints, placeholders, and status labels meet normal-text contrast", () => {
  for (const colors of [light, dark, systemDark]) {
    for (const background of ["canvas", "surface", "surface-alt", "surface-subtle", "nav-tray", "clay-top", "clay-bottom"]) {
      for (const foreground of ["text", "muted"]) {
        assert.ok(contrast(colors[foreground], colors[background]) >= 4.5, `${foreground} on ${background}`);
      }
    }
    for (const background of ["surface", "surface-alt"]) {
      assert.ok(contrast(colors.faint, colors[background]) >= 4.5, `Placeholder on ${background}`);
    }
    for (const status of ["success", "danger", "warning"]) {
      assert.ok(contrast(colors[status], colors[`${status}-bg`]) >= 4.5, `${status} label`);
    }
  }
});

test("essential boundaries and focus remain visible over pastel surfaces and selected-navigation gradients", () => {
  for (const colors of [light, dark, systemDark]) {
    const surfaces = ["canvas", "surface", "surface-alt", "surface-subtle", "nav-tray", "nav-active", "clay-top", "clay-bottom", "pastel-blue", "pastel-lilac", "pastel-mint"];
    const gradientStart = mix(colors["nav-active"], colors["clay-highlight"], .8);
    const backgrounds = [...surfaces.map((name) => [name, colors[name]])];
    for (let step = 0; step <= 20; step++) {
      backgrounds.push([`selected navigation gradient ${step}`, mix(gradientStart, colors["nav-active"], step / 20)]);
    }
    for (const [name, background] of backgrounds) {
      assert.ok(contrast(colors["frame-strong"], background) >= 3, `Essential boundary on ${name}`);
      assert.ok(contrast(colors.focus, background) >= 3, `Keyboard focus on ${name}`);
    }
  }
});
