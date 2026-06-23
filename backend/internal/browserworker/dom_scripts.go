package browserworker

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

func normalizeBrowserURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("url is required")
	}
	if strings.HasPrefix(strings.ToLower(raw), "about:") {
		return raw, nil
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

func resolvePointScript(selector string) string {
	return elementResolverJS() + `
	(function(q){
	const el = jazFindElement(q);
	if (!el) return {found:false};
	el.scrollIntoView({block:"center", inline:"center"});
	const r = el.getBoundingClientRect();
	return {found:true, x:r.left + r.width / 2, y:r.top + r.height / 2, label: jazLabel(el)};
	})` + "(" + jsString(strings.TrimSpace(selector)) + ");"
}

func focusScript(selector string) string {
	return elementResolverJS() + `
	(function(q){
	const el = jazFindElement(q);
	if (!el) return {found:false};
	el.scrollIntoView({block:"center", inline:"center"});
	el.focus();
	return {found:true, label: jazLabel(el)};
	})` + "(" + jsString(strings.TrimSpace(selector)) + ");"
}

func setValueScript(selector, text string, selectOnly bool) string {
	return elementResolverJS() + `
	(function(q, value, selectOnly){
	const el = jazFindElement(q);
	if (!el) return {found:false};
	el.scrollIntoView({block:"center", inline:"center"});
	el.focus();
	let changed = true;
	if (el.tagName === "SELECT") {
	  const needle = jazNorm(value);
	  const option = Array.from(el.options).find(o => jazNorm(o.value) === needle || jazNorm(o.textContent) === needle);
	  if (!option) changed = false; else el.value = option.value;
	} else if (selectOnly) {
	  changed = false;
	} else if (el.isContentEditable) {
	  el.textContent = value;
	} else {
	  el.value = value;
	}
	if (changed) {
	  el.dispatchEvent(new Event("input", {bubbles:true}));
	  el.dispatchEvent(new Event("change", {bubbles:true}));
	}
	return {found:true, changed, label: jazLabel(el)};
	})` + "(" + jsString(strings.TrimSpace(selector)) + "," + jsString(text) + "," + strconv.FormatBool(selectOnly) + ");"
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
	  globalThis.__jazRefMap = new Map();
	  const targets = elements.map((el, i) => {
	    const ref = "e" + (i + 1);
	    globalThis.__jazRefMap.set(ref, el);
	    return {
	      ref,
	      tag: el.tagName.toLowerCase(),
	      role: el.getAttribute("role") || jazImplicitRole(el),
	      name: jazName(el),
	      text: jazShort(el.innerText || el.textContent || el.value || "", 180),
	      href: el.href || "",
	      selector: jazSelector(el)
	    };
	  });
	  return {
	    url: location.href,
	    title: document.title || "",
	    ready_state: document.readyState,
	    text: jazShort(jazPageText(), 3000),
	    elements: targets
	  };
	})()
	`
}

func extractPageScript() string {
	return elementResolverJS() + `
	(function(){
	  const itemLimit = 32;
	  const navLimit = 16;
	  globalThis.__jazRefMap = new Map();
	  const seen = new Set();
	  function cleanText(el, limit){
	    return jazShort(el && (el.innerText || el.textContent || el.value || ""), limit);
	  }
	  function primaryLink(el){
	    if (!el) return null;
	    if (el.matches && el.matches("a[href],button,[role=button],[role=link]")) return el;
	    return jazDeepQuerySelector("a[href],button,[role=button],[role=link]", el);
	  }
	  function titleFor(el, link){
	    const title = jazDeepQuerySelector("h1,h2,h3,[role=heading],strong,b", el);
	    return jazShort((title && cleanText(title, 220)) || jazName(link) || jazName(el), 220);
	  }
	  function candidateSelector(){
	    return [
	      "main article",
	      "main li",
	      "main [role=listitem]",
	      "main tr",
	      "article",
	      "[role=listitem]",
	      "[data-view-name*=search]",
	      "[class*=result]",
	      "[class*=card]",
	      "li",
	      "tr"
	    ].join(",");
	  }
	  function duplicate(el){
	    for (const existing of seen) {
	      if (existing === el || existing.contains(el)) return true;
	    }
	    return false;
	  }
	  const candidates = [];
	  for (const el of jazAll(candidateSelector())) {
	    if (!jazVisible(el) || duplicate(el)) continue;
	    const text = cleanText(el, 900);
	    if (text.length < 16) continue;
	    const link = primaryLink(el);
	    if (!link && text.length < 40) continue;
	    seen.add(el);
	    const refTarget = link || el;
	    const ref = "e" + (candidates.length + 1);
	    globalThis.__jazRefMap.set(ref, refTarget);
	    candidates.push({
	      ref,
	      role: el.getAttribute("role") || jazImplicitRole(el),
	      title: titleFor(el, link),
	      text: text,
	      href: link && link.href ? link.href : ""
	    });
	    if (candidates.length >= itemLimit) break;
	  }
	  const nav = [];
	  const navNeedle = /(next|more|show more|see more|load more|view more|older|newer|\d+\s*\/\s*\d+)/i;
	  for (const el of jazAll("a[href],button,[role=button],[role=link],summary")) {
	    if (!jazVisible(el)) continue;
	    const name = jazName(el);
	    if (!navNeedle.test(name)) continue;
	    const ref = "e" + (candidates.length + nav.length + 1);
	    globalThis.__jazRefMap.set(ref, el);
	    nav.push({
	      ref,
	      tag: el.tagName.toLowerCase(),
	      role: el.getAttribute("role") || jazImplicitRole(el),
	      name,
	      text: cleanText(el, 180),
	      href: el.href || "",
	      selector: jazSelector(el)
	    });
	    if (nav.length >= navLimit) break;
	  }
	  const doc = document.documentElement || document.body;
	  const height = Math.max(doc.scrollHeight || 0, document.body ? document.body.scrollHeight || 0 : 0, innerHeight || 0);
	  const maxScroll = Math.max(1, height - (innerHeight || 0));
	  const scrollYValue = Math.max(0, Math.round(scrollY || pageYOffset || 0));
	  return {
	    url: location.href,
	    title: document.title || "",
	    ready_state: document.readyState,
	    coverage: {
	      scroll_y: scrollYValue,
	      viewport_height: Math.round(innerHeight || 0),
	      document_height: Math.round(height),
	      scroll_percent: Math.max(0, Math.min(100, Math.round((scrollYValue / maxScroll) * 100))),
	      at_bottom: scrollYValue + (innerHeight || 0) >= height - 8
	    },
	    items: candidates,
	    navigation: nav
	  };
	})()
	`
}

func elementResolverJS() string {
	return `
	function jazNorm(s){ return String(s || "").replace(/\s+/g, " ").trim().toLowerCase(); }
	function jazShort(s, n){ s = String(s || "").replace(/\s+/g, " ").trim(); return s.length > n ? s.slice(0, n).trim() : s; }
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
	  const raw = el.getAttribute("aria-label") || el.getAttribute("name") || el.getAttribute("placeholder") || el.getAttribute("title") || el.innerText || el.textContent || el.value || "";
	  return jazShort(raw, 180);
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
	function jazDeepQuerySelector(selector, root){
	  root = root || document;
	  const direct = root.querySelector(selector);
	  if (direct) return direct;
	  for (const host of root.querySelectorAll("*")) {
	    if (!host.shadowRoot) continue;
	    const found = jazDeepQuerySelector(selector, host.shadowRoot);
	    if (found) return found;
	  }
	  return undefined;
	}
	function jazVisible(el){
	  if (!(el instanceof Element)) return false;
	  const style = getComputedStyle(el);
	  if (style.visibility === "hidden" || style.display === "none" || Number(style.opacity) === 0) return false;
	  const r = el.getBoundingClientRect();
	  return r.width > 0 && r.height > 0 && r.bottom >= 0 && r.right >= 0 && r.top <= innerHeight && r.left <= innerWidth;
	}
	function jazPageText(){
	  const chunks = [document.body ? document.body.innerText || document.body.textContent || "" : ""];
	  for (const host of document.querySelectorAll("*")) {
	    if (host.shadowRoot) chunks.push(host.shadowRoot.textContent || "");
	  }
	  return String(chunks.join(" ")).replace(/\s+/g, " ").trim();
	}
	function jazSelector(el){
	  if (!el || !el.tagName) return "";
	  if (el.id) return "#" + CSS.escape(el.id);
	  const tag = el.tagName.toLowerCase();
	  const name = el.getAttribute("name");
	  if (name) return tag + "[name=" + JSON.stringify(name) + "]";
	  return tag;
	}
	function jazFindElement(q){
	  q = String(q || "").trim();
	  if (!q) return document.activeElement;
	  if (q.startsWith("ref=")) {
	    const el = globalThis.__jazRefMap && globalThis.__jazRefMap.get(q.slice(4));
	    if (el && el.isConnected) return el;
	  }
	  try {
	    const selected = jazDeepQuerySelector(q);
	    if (selected) return selected;
	  } catch (_) {}
	  const needle = jazNorm(q);
	  const els = jazAll("a,button,input,textarea,select,label,[role],[aria-label],[title],[placeholder],[contenteditable=true]");
	  return els.find(el => jazNorm(el.getAttribute("aria-label") || el.getAttribute("name") || el.getAttribute("placeholder") || el.getAttribute("title") || el.innerText || el.textContent || el.value) === needle)
	    || els.find(el => jazNorm(el.getAttribute("aria-label") || el.getAttribute("name") || el.getAttribute("placeholder") || el.getAttribute("title") || el.innerText || el.textContent || el.value).includes(needle));
	}
	`
}
