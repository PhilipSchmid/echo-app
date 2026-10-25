let closeActive = null;
const enhanced = new WeakMap();

export function nextOptionIndex(options, current, key, search = "") {
  const enabled = options.flatMap((option, index) => option.disabled ? [] : [index]);
  if (!enabled.length) return -1;
  if (key === "Home") return enabled[0];
  if (key === "End") return enabled[enabled.length - 1];
  if (key === "ArrowDown") return enabled.find((index) => index > current) ?? enabled[enabled.length - 1];
  if (key === "ArrowUp") return enabled.findLast((index) => index < current) ?? enabled[0];
  if (search) {
    const query = search.toLocaleLowerCase();
    const order = enabled.filter((index) => index > current).concat(enabled.filter((index) => index <= current));
    return order.find((index) => options[index].label.toLocaleLowerCase().startsWith(query)) ?? (enabled.includes(current) ? current : enabled[0]);
  }
  return enabled.includes(current) ? current : enabled[0];
}

export function enhanceSelect(select) {
  if (!select || select.multiple || select.size > 1 || !select.options.length) return null;
  if (enhanced.has(select)) return enhanced.get(select);
  const doc = select.ownerDocument;
  const originalOptions = Array.from(select.options);
  let wrapper = select.parentElement;
  if (!wrapper.classList.contains("select-control")) {
    wrapper = doc.createElement("div");
    select.before(wrapper);
    wrapper.append(select);
  }
  wrapper.classList.add("select-control", select.id + "-select");
  const label = select.labels[0];
  const trigger = doc.createElement("button");
  trigger.id = select.id + "-trigger";
  trigger.type = "button";
  trigger.className = "select-trigger";
  trigger.setAttribute("role", "combobox");
  trigger.setAttribute("aria-haspopup", "listbox");
  trigger.setAttribute("aria-expanded", "false");
  if (label) {
    if (!label.id) label.id = select.id + "-label";
    trigger.setAttribute("aria-labelledby", label.id);
  } else {
    trigger.setAttribute("aria-label", select.getAttribute("aria-label") || select.name || select.id);
  }
  if (select.hasAttribute("aria-describedby")) trigger.setAttribute("aria-describedby", select.getAttribute("aria-describedby"));
  const value = doc.createElement("span");
  value.className = "select-label";
  const chevron = doc.createElement("span");
  chevron.className = "select-chevron";
  chevron.setAttribute("aria-hidden", "true");
  trigger.append(value, chevron);
  const menu = doc.createElement("div");
  menu.id = select.id + "-listbox";
  menu.className = "select-menu";
  menu.setAttribute("role", "listbox");
  if (label) menu.setAttribute("aria-labelledby", label.id);
  else menu.setAttribute("aria-label", trigger.getAttribute("aria-label"));
  menu.hidden = true;
  if (typeof menu.showPopover === "function") menu.setAttribute("popover", "manual");
  const choices = originalOptions.map((option, index) => {
    const item = doc.createElement("div");
    item.id = select.id + "-option-" + index;
    item.className = "select-option";
    item.setAttribute("role", "option");
    item.dataset.index = String(index);
    item.textContent = option.label;
    menu.append(item);
    return item;
  });
  wrapper.append(trigger, menu);
  let open = false;
  let active = select.selectedIndex;
  let lastSelected = select.selectedIndex;
  let search = "";
  let lastTyped = 0;
  const options = () => originalOptions.map((option) => ({ label: option.label, disabled: option.disabled || option.hidden || option.parentElement.disabled }));

  function highlight(index) {
    active = index;
    choices.forEach((choice, current) => { choice.dataset.active = String(current === active); });
    if (open && active >= 0) {
      trigger.setAttribute("aria-activedescendant", choices[active].id);
      choices[active].scrollIntoView({ block: "nearest" });
    } else trigger.removeAttribute("aria-activedescendant");
  }
  function refresh() {
    const selectionChanged = lastSelected !== select.selectedIndex;
    lastSelected = select.selectedIndex;
    value.textContent = originalOptions[select.selectedIndex]?.label || "";
    trigger.disabled = select.matches(":disabled");
    trigger.setAttribute("aria-disabled", String(trigger.disabled));
    trigger.setAttribute("aria-invalid", select.getAttribute("aria-invalid") || "false");
    const state = options();
    choices.forEach((choice, index) => {
      choice.setAttribute("aria-selected", String(index === select.selectedIndex));
      choice.setAttribute("aria-disabled", String(state[index].disabled));
      choice.hidden = originalOptions[index].hidden;
    });
    if (trigger.disabled && open) close(false);
    if (!open || selectionChanged) highlight(select.selectedIndex);
  }
  function close(commit) {
    if (!open) return;
    open = false;
    search = "";
    lastTyped = 0;
    if (typeof menu.hidePopover === "function" && menu.matches(":popover-open")) menu.hidePopover();
    menu.hidden = true;
    trigger.setAttribute("aria-expanded", "false");
    trigger.removeAttribute("aria-controls");
    trigger.removeAttribute("aria-activedescendant");
    wrapper.classList.remove("is-open");
    if (closeActive === close) closeActive = null;
    if (commit && active >= 0 && !options()[active].disabled && select.selectedIndex !== active) {
      select.selectedIndex = active;
      select.dispatchEvent(new Event("change", { bubbles: true }));
    }
    refresh();
  }
  function position() {
    if (!open) return;
    const rect = trigger.getBoundingClientRect();
    const gap = 6;
    const margin = 8;
    const width = Math.min(Math.max(rect.width, 176), window.innerWidth - margin * 2);
    menu.style.width = width + "px";
    const below = window.innerHeight - rect.bottom - gap - margin;
    const above = rect.top - gap - margin;
    const upwards = below < Math.min(menu.scrollHeight, 384) && above > below;
    menu.style.maxHeight = Math.max(44, Math.min(384, upwards ? above : below)) + "px";
    menu.style.left = Math.max(margin, Math.min(rect.left, window.innerWidth - width - margin)) + "px";
    menu.style.top = Math.max(margin, Math.min(upwards ? rect.top - gap - menu.offsetHeight : rect.bottom + gap, window.innerHeight - menu.offsetHeight - margin)) + "px";
  }
  function show() {
    refresh();
    if (trigger.disabled) return false;
    if (open) return true;
    if (closeActive && closeActive !== close) closeActive(true);
    open = true;
    closeActive = close;
    menu.hidden = false;
    trigger.setAttribute("aria-controls", menu.id);
    if (typeof menu.showPopover === "function") {
      try { menu.showPopover(); } catch { menu.removeAttribute("popover"); }
    }
    wrapper.classList.add("is-open");
    trigger.setAttribute("aria-expanded", "true");
    position();
    highlight(nextOptionIndex(options(), select.selectedIndex, "current"));
    trigger.focus({ preventScroll: true });
    return true;
  }
  trigger.addEventListener("click", () => { if (open) close(false); else show(); });
  trigger.addEventListener("focus", refresh);
  trigger.addEventListener("blur", () => close(true));
  trigger.addEventListener("keydown", (event) => {
    if (event.isComposing || event.keyCode === 229 || event.ctrlKey || event.metaKey || (event.altKey && event.key !== "ArrowDown" && event.key !== "ArrowUp")) return;
    if (event.key === "Tab") { close(true); return; }
    if (event.key === "Escape") { if (open) { event.preventDefault(); close(false); } return; }
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      if (open) close(true); else show();
      return;
    }
    if (["ArrowDown", "ArrowUp", "Home", "End"].includes(event.key)) {
      event.preventDefault();
      const wasOpen = open;
      if (!show()) return;
      if (event.altKey && event.key === "ArrowUp") { close(true); return; }
      if (wasOpen || event.key === "Home" || event.key === "End") highlight(nextOptionIndex(options(), active, event.key));
      return;
    }
    if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
      event.preventDefault();
      const now = Date.now();
      search = now - lastTyped < 700 ? search + event.key : event.key;
      lastTyped = now;
      const query = Array.from(search).every((character) => character === search[0]) ? search[0] : search;
      if (show()) highlight(nextOptionIndex(options(), active, "typeahead", query));
    }
  });
  menu.addEventListener("mousedown", (event) => { if (event.target.closest(".select-option")) event.preventDefault(); });
  menu.addEventListener("click", (event) => {
    const choice = event.target.closest(".select-option");
    if (!choice) return;
    const index = Number(choice.dataset.index);
    if (options()[index].disabled) return;
    active = index;
    close(true);
    trigger.focus({ preventScroll: true });
  });
  doc.addEventListener("pointerdown", (event) => {
    if (open && !wrapper.contains(event.target) && !menu.contains(event.target)) close(true);
  });
  window.addEventListener("resize", position);
  window.addEventListener("scroll", position, true);
  select.addEventListener("change", refresh);
  refresh();
  if (label) label.htmlFor = trigger.id;
  select.hidden = true;
  const control = { refresh };
  enhanced.set(select, control);
  return control;
}
