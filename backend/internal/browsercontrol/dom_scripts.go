package browsercontrol

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

func normalizeBrowserURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("url is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported browser URL scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return "", errors.New("browser URL requires a host")
	}
	return u.String(), nil
}

func jsString(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func resolvePointScript(ref string) string {
	return elementResolverJS() + `
	(function(q){
	const el = jazFindElement(q);
	if (!el) return {found:false};
	if (el.disabled || el.getAttribute("aria-disabled") === "true") return {found:false, reason:"target is disabled"};
	el.scrollIntoView({block:"center", inline:"center"});
	const r = el.getBoundingClientRect();
	if (r.width <= 0 || r.height <= 0) return {found:false, reason:"target is not visible"};
	const x = r.left + r.width / 2;
	const y = r.top + r.height / 2;
	const hit = jazDeepHit(x, y);
	if (!jazComposedContains(el, hit)) return {found:false, reason:"target is obscured; read the page again"};
	return {found:true, x, y, label: jazLabel(el)};
	})` + "(" + jsString(strings.TrimSpace(ref)) + ");"
}

func focusScript(ref string) string {
	return elementResolverJS() + `
	(function(q){
	const el = jazFindElement(q);
	if (!el) return {found:false};
	el.scrollIntoView({block:"center", inline:"center"});
	el.focus();
	return {found:true, label: jazLabel(el)};
	})` + "(" + jsString(strings.TrimSpace(ref)) + ");"
}

func probeRefScript(ref string) string {
	return elementResolverJS() + `
	(function(q){
	const el = jazFindElement(q);
	if (!el) return {found:false};
	return {found:true, label: jazLabel(el)};
	})` + "(" + jsString(strings.TrimSpace(ref)) + ");"
}

func formInputScript(ref string, value any) string {
	encoded, _ := json.Marshal(value)
	return elementResolverJS() + `
	(function(q, value){
	const el = jazFindElement(q);
	if (!el) return {found:false};
	el.scrollIntoView({block:"center", inline:"center"});
	el.focus();
	let changed = true;
	let reason = "";
	const type = String(el.getAttribute("type") || "").toLowerCase();
	if (el.disabled || el.readOnly || el.getAttribute("aria-disabled") === "true" || el.getAttribute("aria-readonly") === "true") {
	  changed = false;
	  reason = "target is disabled or read-only";
	} else if (el instanceof HTMLInputElement && (type === "checkbox" || type === "radio")) {
	  if (typeof value !== "boolean" || (type === "radio" && !value)) {
	    changed = false;
	    reason = type + " requires " + (type === "radio" ? "true" : "a boolean value");
	  }
	  else {
	    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "checked");
	    if (setter && setter.set) setter.set.call(el, value); else el.checked = value;
	  }
	} else if (el instanceof HTMLSelectElement) {
	  const needle = jazNorm(value);
	  const option = Array.from(el.options).find(o => jazNorm(o.value) === needle || jazNorm(o.textContent) === needle);
	  if (!option || option.disabled) {
	    changed = false;
	    reason = "select option was not found or is disabled";
	  }
	  else {
	    const setter = Object.getOwnPropertyDescriptor(HTMLSelectElement.prototype, "value");
	    if (setter && setter.set) setter.set.call(el, option.value); else el.value = option.value;
	  }
	} else if (el instanceof HTMLInputElement && ["button","submit","reset","file","image"].includes(type)) {
	  changed = false;
	  reason = "input type " + type + " is not editable";
	} else if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) {
	  const proto = el instanceof HTMLInputElement ? HTMLInputElement.prototype : HTMLTextAreaElement.prototype;
	  const setter = Object.getOwnPropertyDescriptor(proto, "value");
	  const previous = el.value;
	  const next = String(value);
	  if (setter && setter.set) setter.set.call(el, next); else el.value = next;
	  if (next && el.value !== next) {
	    if (setter && setter.set) setter.set.call(el, previous); else el.value = previous;
	    changed = false;
	    reason = "browser rejected the form value";
	  }
	} else if (el.isContentEditable) {
	  el.textContent = String(value);
	} else {
	  changed = false;
	  reason = "target is not a supported form control";
	}
	if (changed) {
	  el.dispatchEvent(new InputEvent("input", {bubbles:true, composed:true, inputType:"insertText"}));
	  el.dispatchEvent(new Event("change", {bubbles:true, composed:true}));
	}
	return {found:true, changed, label: jazLabel(el), reason};
	})` + "(" + jsString(strings.TrimSpace(ref)) + "," + string(encoded) + ");"
}

func textPresentScript(text string) string {
	return `
	(function(needle){
	const want = String(needle || "").replace(/\s+/g, " ").trim().toLowerCase();
	const body = document.body ? document.body.innerText || document.body.textContent || "" : "";
	return want === "" || String(body).replace(/\s+/g, " ").trim().toLowerCase().includes(want);
	})` + "(" + jsString(strings.TrimSpace(text)) + ");"
}

func semanticStateScript() string {
	return elementResolverJS() + `
	(function(){
	  const limit = 60;
	  const elements = jazAll("a[href],button,input,textarea,select,label,summary,[role],[aria-label],[title],[placeholder],[contenteditable=true]")
	    .filter(jazVisible)
	    .slice(0, limit);
	  const revision = jazNewRefRevision();
	  const targets = elements.map((el, i) => {
	    const ref = revision + ":e" + (i + 1);
	    globalThis.__jazRefMap.set(ref, el);
	    return jazStateElement(el, ref);
	  });
	  return {
	    url: location.href,
	    title: document.title || "",
	    ready_state: document.readyState,
	    page_revision: revision,
	    text: jazShort(jazPageText(), 3000),
	    elements: targets
	  };
	})()
	`
}

func findElementsScript(query string) string {
	return elementResolverJS() + `
	(function(query){
	  const needle = jazNorm(query);
	  const tokens = needle.split(/[^a-z0-9]+/).filter(token => token.length > 1);
	  const wantsControls = tokens.some(token => ["field","fields","form","control","controls","input","inputs"].includes(token));
	  const elements = jazAll("a[href],button,input,textarea,select,label,summary,[role],[aria-label],[title],[placeholder],[contenteditable=true]")
	    .filter(jazRendered);
	  const ranked = elements.map((el, index) => {
	    const haystack = jazNorm([
	      jazName(el),
	      el.getAttribute("role"),
	      jazImplicitRole(el),
	      el.getAttribute("type"),
	      el.getAttribute("name"),
	      el.getAttribute("placeholder"),
	      el.required ? "required" : "",
	      el.disabled ? "disabled" : "",
	      el.checked ? "checked" : "",
	      el.tagName
	    ].join(" "));
	    let score = haystack === needle ? 100 : 0;
	    if (needle && haystack.includes(needle)) score += 20;
	    for (const token of tokens) if (haystack.includes(token)) score += 3;
	    if (wantsControls && el.matches("input,textarea,select,[contenteditable=true]")) score += 2;
	    return {el, index, score};
	  }).filter(item => item.score > 0)
	    .sort((a, b) => b.score - a.score || a.index - b.index)
	    .slice(0, wantsControls ? 60 : 20);
	  const revision = jazNewRefRevision();
	  const targets = ranked.map((item, i) => {
	    const ref = revision + ":e" + (i + 1);
	    globalThis.__jazRefMap.set(ref, item.el);
	    return jazStateElement(item.el, ref);
	  });
	  return {
	    url: location.href,
	    title: document.title || "",
	    ready_state: document.readyState,
	    page_revision: revision,
	    text: targets.length ? "Matches for " + JSON.stringify(query) : "No elements matched " + JSON.stringify(query),
	    elements: targets
	  };
	})` + "(" + jsString(strings.TrimSpace(query)) + ");"
}

func elementResolverJS() string {
	return `
	function jazNorm(s){ return String(s || "").replace(/\s+/g, " ").trim().toLowerCase(); }
	function jazShort(s, n){ s = String(s || "").replace(/\s+/g, " ").trim(); return s.length > n ? s.slice(0, n).trim() : s; }
	function jazPageRevision(){
	  if (!globalThis.__jazRevisionState) {
	    const state = {value: 1};
	    new MutationObserver(() => { state.value += 1; }).observe(document, {
	      subtree:true,
	      childList:true,
	      attributes:true,
	      attributeFilter:["id","role","aria-label","aria-labelledby","aria-hidden","aria-disabled","aria-checked","aria-required","disabled","hidden","name","type","placeholder","required"]
	    });
	    globalThis.__jazRevisionState = state;
	  }
	  return "p" + globalThis.__jazRevisionState.value;
	}
	function jazNewRefRevision(){
	  jazPageRevision();
	  globalThis.__jazRevisionState.value += 1;
	  globalThis.__jazRefMap = new Map();
	  return jazPageRevision();
	}
	function jazLabel(el){
	  const raw = jazName(el) || el.tagName;
	  return el.tagName.toLowerCase() + " " + JSON.stringify(String(raw || "").replace(/\s+/g, " ").trim().slice(0, 120));
	}
	function jazName(el){
	  if (!el) return "";
	  const labelledBy = el.getAttribute("aria-labelledby");
	  if (labelledBy) {
	    const root = el.getRootNode && el.getRootNode();
	    const labels = labelledBy.split(/\s+/).map(id => (root && root.getElementById ? root.getElementById(id) : null) || document.getElementById(id)).filter(Boolean).map(label => label.textContent);
	    if (labels.length) return jazShort(labels.join(" "), 180);
	  }
	  const password = el instanceof HTMLInputElement && String(el.getAttribute("type") || "").toLowerCase() === "password";
	  const raw = el.getAttribute("aria-label") || el.getAttribute("name") || el.getAttribute("placeholder") || el.getAttribute("title") || el.innerText || el.textContent || (password ? "" : el.value) || "";
	  return jazShort(raw, 180);
	}
	function jazStateElement(el, ref){
	  const tag = el.tagName.toLowerCase();
	  const inputType = tag === "input" ? String(el.getAttribute("type") || "text").toLowerCase() : "";
	  const sensitive = inputType === "password";
	  const value = sensitive ? "" : ("value" in el ? String(el.value || "") : "");
	  return {
	    ref,
	    tag,
	    role: el.getAttribute("role") || jazImplicitRole(el),
	    name: jazName(el),
	    text: jazShort(el.innerText || el.textContent || "", 180),
	    href: el.href || "",
	    input_type: inputType,
	    value,
	    required: Boolean(el.required || el.getAttribute("aria-required") === "true"),
	    disabled: Boolean(el.disabled || el.getAttribute("aria-disabled") === "true"),
	    read_only: Boolean(el.readOnly || el.getAttribute("aria-readonly") === "true"),
	    checked: Boolean(el.checked || el.getAttribute("aria-checked") === "true"),
	    validation: jazShort(el.validationMessage || "", 180)
	  };
	}
	function jazImplicitRole(el){
	  const tag = el.tagName ? el.tagName.toLowerCase() : "";
	  if (tag === "a" && el.hasAttribute("href")) return "link";
	  if (tag === "button") return "button";
	  if (tag === "textarea") return "textbox";
	  if (tag === "select") return "combobox";
	  if (tag === "input") {
	    const type = String(el.getAttribute("type") || "text").toLowerCase();
	    if (["button","submit","reset"].includes(type)) return "button";
	    if (["checkbox","radio","range"].includes(type)) return type === "range" ? "slider" : type;
	    return "textbox";
	  }
	  return "";
	}
	function jazAll(selector, root, out){
	  root = root || document;
	  out = out || [];
	  out.push(...root.querySelectorAll(selector));
	  for (const host of root.querySelectorAll("*")) {
	    if (host.shadowRoot) jazAll(selector, host.shadowRoot, out);
	  }
	  return out;
	}
	function jazVisible(el){
	  if (!(el instanceof Element)) return false;
	  const style = getComputedStyle(el);
	  if (style.visibility === "hidden" || style.display === "none" || Number(style.opacity) === 0) return false;
	  const r = el.getBoundingClientRect();
	  return r.width > 0 && r.height > 0 && r.bottom >= 0 && r.right >= 0 && r.top <= innerHeight && r.left <= innerWidth;
	}
	function jazRendered(el){
	  if (!(el instanceof Element)) return false;
	  const style = getComputedStyle(el);
	  if (style.visibility === "hidden" || style.display === "none" || Number(style.opacity) === 0) return false;
	  const r = el.getBoundingClientRect();
	  return r.width > 0 && r.height > 0;
	}
	function jazPageText(){
	  const chunks = [document.body ? document.body.innerText || document.body.textContent || "" : ""];
	  for (const host of document.querySelectorAll("*")) {
	    if (host.shadowRoot) chunks.push(host.shadowRoot.textContent || "");
	  }
	  return String(chunks.join(" ")).replace(/\s+/g, " ").trim();
	}
	function jazDeepHit(x, y){
	  let hit = document.elementFromPoint(x, y);
	  while (hit && hit.shadowRoot && hit.shadowRoot.elementFromPoint) {
	    const inner = hit.shadowRoot.elementFromPoint(x, y);
	    if (!inner || inner === hit) break;
	    hit = inner;
	  }
	  return hit;
	}
	function jazComposedContains(ancestor, node){
	  for (let current = node; current;) {
	    if (current === ancestor) return true;
	    if (current.assignedSlot) {
	      current = current.assignedSlot;
	      continue;
	    }
	    if (current.parentNode) {
	      current = current.parentNode;
	      continue;
	    }
	    const root = current.getRootNode ? current.getRootNode() : null;
	    current = root instanceof ShadowRoot ? root.host : null;
	  }
	  return false;
	}
	function jazFindElement(q){
	  q = String(q || "").trim();
	  if (!q) return document.activeElement;
	  {
	    const ref = q.startsWith("ref=") ? q.slice(4) : q;
	    const revision = ref.split(":", 1)[0];
	    if (revision.startsWith("p") && revision !== jazPageRevision()) return undefined;
	    const el = globalThis.__jazRefMap && globalThis.__jazRefMap.get(ref);
	    if (el && el.isConnected) return el;
	  }
	  return undefined;
	}
	`
}
