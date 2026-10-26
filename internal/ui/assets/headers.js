import { element, headersFromRows, maxHeaderRows, maxHeaderCharacters } from "./common.js";

export function createHeaderEditor(container, addButton, onChange) {
  let rows = [];
  function renumber() {
    rows.forEach((row, index) => {
      row.key.setAttribute("aria-label", "Header name " + (index + 1));
      row.value.setAttribute("aria-label", "Header value " + (index + 1));
      row.remove.setAttribute("aria-label", "Remove header " + (index + 1));
      row.remove.setAttribute("data-tooltip", "Remove header " + (index + 1));
    });
    addButton.disabled = rows.length >= maxHeaderRows;
  }
  function append(name = "", value = "") {
    const node = element("div", "header-row");
    const keyField = element("label", "header-key-field", "Name");
    const valueField = element("label", "header-value-field", "Value");
    const key = element("input", "header-key mono");
    const input = element("input", "header-value mono");
    for (const field of [key, input]) {
      field.type = "text";
      field.autocomplete = "off";
      field.spellcheck = false;
      field.setAttribute("autocapitalize", "none");
      field.setAttribute("aria-describedby", "headers-hint");
      field.maxLength = maxHeaderCharacters;
    }
    key.value = name;
    key.placeholder = "X-Example";
    input.value = value;
    input.placeholder = "Header value";
    const remove = element("button", "button secondary header-remove");
    const removeIcon = element("span", "", "×");
    removeIcon.setAttribute("aria-hidden", "true");
    remove.append(removeIcon);
    remove.type = "button";
    const row = { node, key, value: input, remove };
    remove.addEventListener("click", () => {
      const index = rows.indexOf(row);
      if (rows.length === 1) { key.value = ""; input.value = ""; }
      else { rows.splice(index, 1); node.remove(); }
      renumber();
      rows[Math.min(index, rows.length - 1)].key.focus();
      onChange();
    });
    keyField.append(key);
    valueField.append(input);
    node.append(keyField, valueField, remove);
    container.append(node);
    rows.push(row);
    renumber();
    return row;
  }
  addButton.addEventListener("click", () => {
    if (rows.length >= maxHeaderRows) return;
    append().key.focus();
    onChange();
  });
  function set(values) {
    rows = [];
    container.replaceChildren();
    for (const [name, value] of Object.entries(values)) append(name, value);
    if (!rows.length) append();
  }
  set({});
  return {
    set,
    read() {
      try { return headersFromRows(rows.map(({ key, value }) => ({ name: key.value, value: value.value }))); }
      catch (error) {
        const row = rows[error.row] || rows[0];
        error.field = error.part === "name" ? row.key : row.value;
        throw error;
      }
    }
  };
}
